#!/usr/bin/env bash

set -euo pipefail

# ============================================================
# OpenVPN Community Server Installer
# Ubuntu 22.04 / 24.04
#
# 功能：
# - 安装 OpenVPN Community
# - 安装 EasyRSA
# - 初始化 CA / Server Certificate
# - 创建 tls-crypt key
# - 创建 server.conf
# - 创建 routes.conf
# - 开启 IPv4 Forward
# - 配置 NAT
# - 启动 OpenVPN
#
# 不做：
# - 不创建客户端
# - 不生成 .ovpn
# - 不要求公网 IP
# ============================================================

if [ "$(id -u)" -ne 0 ]; then
    echo "请使用 root 运行此脚本"
    exit 1
fi

VPN_NETWORK="10.8.0.0"
VPN_NETMASK="255.255.255.0"
VPN_CIDR="10.8.0.0/24"

OPENVPN_PORT="1194"
OPENVPN_PROTO="udp"

EASYRSA_DIR="/etc/openvpn/easy-rsa"
SERVER_DIR="/etc/openvpn/server"
LOG_DIR="/var/log/openvpn"

echo "========================================"
echo " OpenVPN Community Server Installer"
echo "========================================"
echo "VPN 网段 : ${VPN_CIDR}"
echo "VPN 端口 : ${OPENVPN_PORT}/${OPENVPN_PROTO}"
echo "========================================"

# ------------------------------------------------------------
# 1. 安装软件
# ------------------------------------------------------------

echo "[1/8] 安装 OpenVPN / EasyRSA..."

export DEBIAN_FRONTEND=noninteractive

apt-get update

apt-get install -y \
    openvpn \
    easy-rsa \
    iptables \
    iptables-persistent

# ------------------------------------------------------------
# 2. 获取默认出口网卡
# ------------------------------------------------------------

echo "[2/8] 检测默认公网出口网卡..."

WAN_IF=$(ip route get 1.1.1.1 | awk '
{
    for (i=1;i<=NF;i++) {
        if ($i=="dev") {
            print $(i+1)
            exit
        }
    }
}')

if [ -z "${WAN_IF}" ]; then
    echo "无法检测默认出口网卡"
    exit 1
fi

echo "检测到出口网卡: ${WAN_IF}"

# ------------------------------------------------------------
# 3. 创建目录
# ------------------------------------------------------------

echo "[3/8] 创建目录..."

mkdir -p "${SERVER_DIR}"
mkdir -p "${EASYRSA_DIR}"
mkdir -p "${LOG_DIR}"

chmod 700 "${EASYRSA_DIR}"

# ------------------------------------------------------------
# 4. 初始化 EasyRSA / PKI
# ------------------------------------------------------------

echo "[4/8] 初始化 PKI..."

if [ ! -f "${EASYRSA_DIR}/easyrsa" ]; then
    cp -r /usr/share/easy-rsa/* "${EASYRSA_DIR}/"
fi

cd "${EASYRSA_DIR}"

if [ ! -d "${EASYRSA_DIR}/pki" ]; then
    ./easyrsa init-pki
fi

if [ ! -f "${EASYRSA_DIR}/pki/ca.crt" ]; then
    EASYRSA_BATCH=1 ./easyrsa build-ca nopass
fi

# ------------------------------------------------------------
# 5. 创建 Server Certificate / CRL / tls-crypt
# ------------------------------------------------------------

echo "[5/8] 创建服务器证书..."

if [ ! -f "${EASYRSA_DIR}/pki/issued/server.crt" ]; then
    EASYRSA_BATCH=1 ./easyrsa build-server-full server nopass
fi

EASYRSA_BATCH=1 ./easyrsa gen-crl

cp "${EASYRSA_DIR}/pki/ca.crt" \
   "${SERVER_DIR}/ca.crt"

cp "${EASYRSA_DIR}/pki/issued/server.crt" \
   "${SERVER_DIR}/server.crt"

cp "${EASYRSA_DIR}/pki/private/server.key" \
   "${SERVER_DIR}/server.key"

cp "${EASYRSA_DIR}/pki/crl.pem" \
   "${SERVER_DIR}/crl.pem"

chmod 600 "${SERVER_DIR}/server.key"
chmod 644 "${SERVER_DIR}/crl.pem"

if [ ! -f "${SERVER_DIR}/tls-crypt.key" ]; then
    openvpn --genkey secret "${SERVER_DIR}/tls-crypt.key"
fi

chmod 600 "${SERVER_DIR}/tls-crypt.key"

# ------------------------------------------------------------
# 6. 创建 Split Tunnel 路由文件
# ------------------------------------------------------------

echo "[6/8] 创建 Split Tunnel 路由配置..."

if [ ! -f "${SERVER_DIR}/routes.conf" ]; then

cat > "${SERVER_DIR}/routes.conf" <<'EOF'
# ============================================================
# OpenVPN Split Tunnel Routes
#
# 只有这里配置的 IP / CIDR 才会经过 VPN。
#
# 示例：
#
# push "route 8.8.8.8 255.255.255.255"
# push "route 1.2.3.4 255.255.255.255"
# push "route 20.30.40.0 255.255.255.0"
#
# 注意：
#
# 不要添加：
#
# push "redirect-gateway def1"
#
# 否则客户端所有流量都会经过 VPN。
# ============================================================
EOF

fi

# ------------------------------------------------------------
# 7. 创建 OpenVPN Server 配置
# ------------------------------------------------------------

echo "[7/8] 创建 server.conf..."

cat > "${SERVER_DIR}/server.conf" <<EOF
# ============================================================
# OpenVPN Community Server
# ============================================================

port ${OPENVPN_PORT}
proto ${OPENVPN_PROTO}
dev tun

topology subnet

server ${VPN_NETWORK} ${VPN_NETMASK}


# ----------------------------
# PKI
# ----------------------------

ca ${SERVER_DIR}/ca.crt
cert ${SERVER_DIR}/server.crt
key ${SERVER_DIR}/server.key

dh none

crl-verify ${SERVER_DIR}/crl.pem

tls-crypt ${SERVER_DIR}/tls-crypt.key

tls-version-min 1.2


# ----------------------------
# Encryption
# ----------------------------

data-ciphers AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305

auth SHA256


# ----------------------------
# Split Tunnel
# ----------------------------

config ${SERVER_DIR}/routes.conf


# ----------------------------
# Connection
# ----------------------------

keepalive 10 120

persist-key
persist-tun


# ----------------------------
# Online Users / Web UI
# ----------------------------

status ${LOG_DIR}/status.log 10
status-version 3


# ----------------------------
# Security
# ----------------------------

user nobody
group nogroup


# ----------------------------
# Logs
# ----------------------------

verb 3
EOF

# ------------------------------------------------------------
# 8. 开启转发 + NAT
# ------------------------------------------------------------

echo "[8/8] 配置 Forward / NAT..."

cat > /etc/sysctl.d/99-openvpn.conf <<EOF
net.ipv4.ip_forward=1
EOF

sysctl --system >/dev/null

iptables -t nat -C POSTROUTING \
    -s "${VPN_CIDR}" \
    -o "${WAN_IF}" \
    -j MASQUERADE 2>/dev/null || \
iptables -t nat -A POSTROUTING \
    -s "${VPN_CIDR}" \
    -o "${WAN_IF}" \
    -j MASQUERADE

iptables -C FORWARD \
    -s "${VPN_CIDR}" \
    -o "${WAN_IF}" \
    -j ACCEPT 2>/dev/null || \
iptables -A FORWARD \
    -s "${VPN_CIDR}" \
    -o "${WAN_IF}" \
    -j ACCEPT

iptables -C FORWARD \
    -d "${VPN_CIDR}" \
    -i "${WAN_IF}" \
    -m conntrack \
    --ctstate ESTABLISHED,RELATED \
    -j ACCEPT 2>/dev/null || \
iptables -A FORWARD \
    -d "${VPN_CIDR}" \
    -i "${WAN_IF}" \
    -m conntrack \
    --ctstate ESTABLISHED,RELATED \
    -j ACCEPT

netfilter-persistent save

# ------------------------------------------------------------
# 启动 OpenVPN
# ------------------------------------------------------------

systemctl enable openvpn-server@server
systemctl restart openvpn-server@server

echo ""
echo "========================================"
echo " OpenVPN Server 安装完成"
echo "========================================"
echo ""
echo "VPN 网段:"
echo "  ${VPN_CIDR}"
echo ""
echo "监听:"
echo "  ${OPENVPN_PORT}/${OPENVPN_PROTO}"
echo ""
echo "出口网卡:"
echo "  ${WAN_IF}"
echo ""
echo "主配置:"
echo "  ${SERVER_DIR}/server.conf"
echo ""
echo "分流路由配置:"
echo "  ${SERVER_DIR}/routes.conf"
echo ""
echo "PKI:"
echo "  ${EASYRSA_DIR}/pki"
echo ""
echo "在线用户状态:"
echo "  ${LOG_DIR}/status.log"
echo ""
echo "查看服务:"
echo "  systemctl status openvpn-server@server"
echo ""
echo "查看日志:"
echo "  journalctl -u openvpn-server@server -f"
echo ""
echo "重要："
echo "  请确认云服务器安全组已开放 UDP ${OPENVPN_PORT}"
echo ""
echo "========================================"