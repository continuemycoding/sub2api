package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeMaxReasoningEffort(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "separator", in: "x-high", want: "xhigh"},
		{name: "max is distinct", in: "max", want: "max"},
		{name: "none is unsupported", in: "none", want: ""},
		{name: "invalid", in: "banana", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeMaxReasoningEffort(tt.in))
		})
	}
}

func TestNormalizeReasoningEffortMappings(t *testing.T) {
	t.Run("canonicalizes fixed OpenAI values", func(t *testing.T) {
		got, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: " MAX ", To: " x-high "},
			{From: "minimal", To: "high"},
		})
		require.NoError(t, err)
		require.Equal(t, []ReasoningEffortMapping{
			{From: "max", To: "xhigh"},
			{From: "minimal", To: "high"},
		}, got)
	})

	t.Run("rejects empty values", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "max"}})
		require.ErrorContains(t, err, "empty or unknown")
	})

	t.Run("rejects duplicate sources case insensitively", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "max", To: "xhigh"},
			{From: " MAX ", To: "high"},
		})
		require.ErrorContains(t, err, "duplicate")
	})

	t.Run("rejects mappings for non OpenAI platforms", func(t *testing.T) {
		for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrok} {
			_, err := NormalizeReasoningEffortMappings(platform, []ReasoningEffortMapping{{From: "low", To: "high"}})
			require.ErrorContains(t, err, "only supported for platform \"openai\"")
		}

		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "none", To: "low"}})
		require.ErrorContains(t, err, "empty or unknown")

		_, err = NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "ultra", To: "high"}})
		require.ErrorContains(t, err, "empty or unknown")
	})
}

func TestNormalizeMaxReasoningEffortForPlatform(t *testing.T) {
	value, err := normalizeMaxReasoningEffortForPlatform(PlatformOpenAI, "max")
	require.NoError(t, err)
	require.Equal(t, "max", value)

	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrok} {
		_, err = normalizeMaxReasoningEffortForPlatform(platform, "low")
		require.ErrorContains(t, err, "only supported for platform \"openai\"")
	}

	_, err = normalizeMaxReasoningEffortForPlatform(PlatformOpenAI, "none")
	require.ErrorContains(t, err, "not supported")
}

func TestApplyOpenAIReasoningEffortModelSuffix(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		wantModel       string
		wantEffort      string
		changed         bool
		chatCompletions bool
	}{
		{name: "sol high", body: `{"model":"gpt-5.6-sol-high","input":"hello"}`, wantModel: "gpt-5.6-sol", wantEffort: "high", changed: true},
		{name: "sol max", body: `{"model":"gpt-5.6-sol-max","input":"hello"}`, wantModel: "gpt-5.6-sol", wantEffort: "max", changed: true},
		{name: "chat completions uses flat effort", body: `{"model":"gpt-5.6-sol-medium","messages":[]}`, wantModel: "gpt-5.6-sol", wantEffort: "medium", changed: true, chatCompletions: true},
		{name: "path alias xhigh", body: `{"model":"openai/gpt-5.6-sol-xhigh"}`, wantModel: "openai/gpt-5.6-sol", wantEffort: "xhigh", changed: true},
		{name: "explicit nested wins", body: `{"model":"gpt-5.6-sol-high","reasoning":{"effort":"low"}}`, wantModel: "gpt-5.6-sol", wantEffort: "low", changed: true},
		{name: "explicit flat wins", body: `{"model":"gpt-5.6-sol-high","reasoning_effort":"medium"}`, wantModel: "gpt-5.6-sol", wantEffort: "medium", changed: true},
		{name: "base model unchanged", body: `{"model":"gpt-5.6-sol"}`, wantModel: "gpt-5.6-sol", changed: false},
		{name: "non OpenAI model unchanged", body: `{"model":"claude-sonnet-high"}`, wantModel: "claude-sonnet-high", changed: false},
		{name: "unknown suffix unchanged", body: `{"model":"gpt-5.6-sol-fast"}`, wantModel: "gpt-5.6-sol-fast", changed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := ApplyOpenAIReasoningEffortModelSuffix([]byte(tt.body), !tt.chatCompletions)
			require.Equal(t, tt.changed, changed)
			require.Equal(t, tt.wantModel, gjson.GetBytes(got, "model").String())
			if tt.wantEffort != "" {
				actual := gjson.GetBytes(got, "reasoning.effort").String()
				if actual == "" {
					actual = gjson.GetBytes(got, "reasoning_effort").String()
				}
				require.Equal(t, tt.wantEffort, actual)
			}
		})
	}
}

func TestOpenAIReasoningEffortModelSuffixRunsBeforePolicy(t *testing.T) {
	body, changed := ApplyOpenAIReasoningEffortModelSuffix([]byte(`{"model":"gpt-5.6-sol-xhigh"}`), true)
	require.True(t, changed)

	body, changed = ApplyOpenAIReasoningEffortPolicy(
		body,
		"medium",
		[]ReasoningEffortMapping{{From: "xhigh", To: "high"}},
	)
	require.True(t, changed)
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(body, "model").String())
	require.Equal(t, "medium", gjson.GetBytes(body, "reasoning.effort").String())
}

func TestApplyOpenAIReasoningEffortPolicy(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		max      string
		mappings []ReasoningEffortMapping
		path     string
		want     string
		changed  bool
	}{
		{name: "nested caps high", body: `{"reasoning":{"effort":"xhigh"}}`, max: "medium", path: "reasoning.effort", want: "medium", changed: true},
		{name: "flat caps high", body: `{"reasoning_effort":"high"}`, max: "low", path: "reasoning_effort", want: "low", changed: true},
		{name: "does not raise omitted", body: `{"model":"gpt-5"}`, max: "low", path: "reasoning_effort", want: "", changed: false},
		{name: "keeps lower value", body: `{"reasoning_effort":"low"}`, max: "high", path: "reasoning_effort", want: "low", changed: false},
		{name: "normalizes request alias", body: `{"reasoning_effort":"x-high"}`, max: "xhigh", path: "reasoning_effort", want: "xhigh", changed: true},
		{name: "caps max below its distinct rank", body: `{"reasoning_effort":"max"}`, max: "xhigh", path: "reasoning_effort", want: "xhigh", changed: true},
		{name: "keeps xhigh below max", body: `{"reasoning_effort":"xhigh"}`, max: "max", path: "reasoning_effort", want: "xhigh", changed: false},
		{name: "ignores stale none ceiling", body: `{"reasoning_effort":"high"}`, max: "none", path: "reasoning_effort", want: "high", changed: false},
		{name: "caps both shapes", body: `{"reasoning":{"effort":"high"},"reasoning_effort":"xhigh"}`, max: "low", path: "reasoning.effort", want: "low", changed: true},
		{name: "maps before cap", body: `{"reasoning":{"effort":"MAX"}}`, max: "medium", mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"}}, path: "reasoning.effort", want: "medium", changed: true},
		{name: "does not chain mappings", body: `{"reasoning_effort":"max"}`, mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"}, {From: "xhigh", To: "low"}}, path: "reasoning_effort", want: "xhigh", changed: true},
		{name: "keeps unknown without mapping", body: `{"reasoning_effort":"future"}`, max: "low", path: "reasoning_effort", want: "future", changed: false},
		{name: "keeps non string value", body: `{"reasoning_effort":{"level":"high"}}`, max: "low", path: "reasoning_effort.level", want: "high", changed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := ApplyOpenAIReasoningEffortPolicy([]byte(tt.body), tt.max, tt.mappings)
			require.Equal(t, tt.changed, changed)
			if tt.path != "" {
				require.Equal(t, tt.want, gjson.GetBytes(got, tt.path).String())
			}
		})
	}
}
