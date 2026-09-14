#!/usr/bin/env bash
set -euo pipefail
# 在目标 Ubuntu 服务器执行；不会安装或重启 OpenVPN。
if [[ $EUID -ne 0 ]]; then echo '请使用 sudo 运行安装脚本' >&2; exit 1; fi
base=$(cd -- "$(dirname -- "$0")" && pwd)
binary=${1:-"$base/../bin/vpn-admin-linux-amd64"}
[[ -f "$binary" ]] || { echo '缺少 Go 可执行文件' >&2; exit 1; }
# 不覆盖用户可能已安装的其他 ovpn-cli。
if [[ -e /usr/local/bin/ovpn-cli || -L /usr/local/bin/ovpn-cli ]]; then
  [[ -L /usr/local/bin/ovpn-cli && $(readlink /usr/local/bin/ovpn-cli) == /usr/local/bin/vpn-admin ]] || { echo '已有其他 ovpn-cli，请先确认命令归属' >&2; exit 1; }
fi
command -v sudo >/dev/null
command -v visudo >/dev/null
visudo -cf "$base/vpn-admin.sudoers"
id vpn-admin >/dev/null 2>&1 || useradd --system --home /var/lib/vpn-admin --shell /usr/sbin/nologin vpn-admin
install -d -m 0700 -o vpn-admin -g vpn-admin /var/lib/vpn-admin
install -d -m 0755 /etc/vpn-admin /usr/local/libexec
# 升级时先停止 WebUI，避免替换正在使用的 helper。
systemctl stop vpn-admin.service 2>/dev/null || true
install -m 0755 -o root -g root "$binary" /usr/local/bin/vpn-admin
install -m 0755 -o root -g root "$binary" /usr/local/libexec/vpn-admin-helper
ln -sfn /usr/local/bin/vpn-admin /usr/local/bin/ovpn-cli
if [[ ! -e /etc/vpn-admin/config.json ]]; then install -m 0644 -o root -g root "$base/config.example.json" /etc/vpn-admin/config.json; fi
install -m 0440 -o root -g root "$base/vpn-admin.sudoers" /etc/sudoers.d/vpn-admin
install -m 0644 "$base/vpn-admin.tmpfiles" /etc/tmpfiles.d/vpn-admin.conf
install -m 0644 "$base/vpn-admin.service" /etc/systemd/system/vpn-admin.service
systemd-tmpfiles --create /etc/tmpfiles.d/vpn-admin.conf
systemctl daemon-reload
systemctl enable vpn-admin.service
printf '%s\n' '安装完成。请检查 config.json、OpenVPN 主配置、HTTPS 反代后执行 systemctl start vpn-admin。' '首次初始化前，请暂时限制管理站点仅你的 IP 可访问。'
