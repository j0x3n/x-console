#!/bin/sh
# X Console 代理卸载脚本：停止并删除服务，删除程序和配置。
# 面板里这台机器的记录不会自动删除，请在面板的设备页里吊销。
set -eu

[ "$(id -u)" -eq 0 ] || { echo "需要 root 权限，请用 sudo 运行" >&2; exit 1; }

if command -v systemctl >/dev/null 2>&1; then
    systemctl stop x-console-agent >/dev/null 2>&1 || true
    systemctl disable x-console-agent >/dev/null 2>&1 || true
    rm -f /etc/systemd/system/x-console-agent.service
    systemctl daemon-reload >/dev/null 2>&1 || true
fi
rm -f /usr/local/bin/x-console-agent /usr/local/bin/x-console-agent.new
rm -rf /etc/x-console-agent
echo "代理已卸载。请在面板的设备页里吊销这台机器。"
