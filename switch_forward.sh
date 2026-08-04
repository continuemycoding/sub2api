#!/bin/bash
# 端口映射切换脚本：8080 -> 目标IP:8080
# 换IP时只需修改下面 NEW_IP，首次运行把 OLD_IP 留空即可
# 特性：重复执行同一个IP不会产生重复规则（幂等）

OLD_IP="192.168.1.200"                # 旧IP，没有就留空（首次运行场景）
NEW_IP="120.77.176.120"   # 新的目标机器IP
PORT_LOCAL=8080
PORT_REMOTE=8080

# ---------- 1. 确保 IP 转发是开启的 ----------
echo "检查并开启 IP 转发..."
sudo sysctl -w net.ipv4.ip_forward=1 > /dev/null
sudo sed -i '/^net.ipv4.ip_forward/d' /etc/sysctl.conf
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf > /dev/null

# ---------- 2. 删除旧规则（如果指定了OLD_IP）----------
if [ -n "$OLD_IP" ] && [ "$OLD_IP" != "$NEW_IP" ]; then
    echo "删除旧规则（$OLD_IP）..."
    sudo iptables -t nat -D PREROUTING -p tcp --dport $PORT_LOCAL -j DNAT --to-destination $OLD_IP:$PORT_REMOTE 2>/dev/null
    sudo iptables -t nat -D POSTROUTING -p tcp -d $OLD_IP --dport $PORT_REMOTE -j MASQUERADE 2>/dev/null
    sudo iptables -D FORWARD -p tcp -d $OLD_IP --dport $PORT_REMOTE -j ACCEPT 2>/dev/null
    sudo iptables -D FORWARD -p tcp -s $OLD_IP --sport $PORT_REMOTE -j ACCEPT 2>/dev/null
fi

# ---------- 3. 添加新规则（先检查是否已存在，避免重复添加）----------
# iptables -C 用来检查某条规则是否已经存在，
# 存在则返回0（跳过添加），不存在返回非0（才执行添加）
# 这样无论OLD_IP有没有正确填写，重复跑这个脚本都不会产生重复规则

echo "配置 nat PREROUTING 规则..."
sudo iptables -t nat -C PREROUTING -p tcp --dport $PORT_LOCAL -j DNAT --to-destination $NEW_IP:$PORT_REMOTE 2>/dev/null \
    || sudo iptables -t nat -A PREROUTING -p tcp --dport $PORT_LOCAL -j DNAT --to-destination $NEW_IP:$PORT_REMOTE

echo "配置 nat POSTROUTING 规则..."
sudo iptables -t nat -C POSTROUTING -p tcp -d $NEW_IP --dport $PORT_REMOTE -j MASQUERADE 2>/dev/null \
    || sudo iptables -t nat -A POSTROUTING -p tcp -d $NEW_IP --dport $PORT_REMOTE -j MASQUERADE

echo "配置 FORWARD 放行规则..."
sudo iptables -C FORWARD -p tcp -d $NEW_IP --dport $PORT_REMOTE -j ACCEPT 2>/dev/null \
    || sudo iptables -I FORWARD -p tcp -d $NEW_IP --dport $PORT_REMOTE -j ACCEPT
sudo iptables -C FORWARD -p tcp -s $NEW_IP --sport $PORT_REMOTE -j ACCEPT 2>/dev/null \
    || sudo iptables -I FORWARD -p tcp -s $NEW_IP --sport $PORT_REMOTE -j ACCEPT

# ---------- 4. ufw 放行（ufw本身对重复规则会自动跳过，天然幂等）----------
if command -v ufw > /dev/null 2>&1 && sudo ufw status | grep -q "Status: active"; then
    echo "检测到 ufw 已启用，放行本地端口 $PORT_LOCAL..."
    sudo ufw allow $PORT_LOCAL/tcp > /dev/null
fi

# ---------- 5. 持久化保存 ----------
echo "保存规则，确保重启后依然生效..."
sudo netfilter-persistent save > /dev/null 2>&1 || {
    echo "未检测到 netfilter-persistent，正在安装..."
    sudo apt install -y iptables-persistent > /dev/null 2>&1
    sudo netfilter-persistent save
}

echo "完成：$PORT_LOCAL 端口现在转发到 $NEW_IP:$PORT_REMOTE"
echo "注意：请自行检查云服务器安全组是否放行了 $PORT_LOCAL 端口"