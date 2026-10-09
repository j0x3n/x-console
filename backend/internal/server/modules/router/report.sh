#!/bin/sh
# X Console 路由器上报脚本（B114）。由 cron 每分钟运行一次。
# 它把路由器的状态拼成一份纯文本，POST 给面板。面板不需要访问路由器。
URL='@URL@'
TOKEN='@TOKEN@'
TMP=/tmp/xc-report.body

{
	echo '#xc:time'
	date +%s
	echo '#xc:board'
	ubus call system board
	echo '#xc:info'
	ubus call system info
	echo '#xc:interfaces'
	ubus call network.interface dump
	echo '#xc:devices'
	ubus call network.device status
	echo '#xc:leases'
	cat /tmp/dhcp.leases 2>/dev/null
	echo '#xc:arp'
	cat /proc/net/arp 2>/dev/null
} > "$TMP" || exit 1

if command -v curl >/dev/null 2>&1; then
	curl -fsS -m 20 -H "Authorization: Bearer $TOKEN" -H 'Content-Type: text/plain' \
		--data-binary @"$TMP" "$URL" > /dev/null
else
	uclient-fetch -q -T 20 --header="Authorization: Bearer $TOKEN" --header='Content-Type: text/plain' \
		--post-file="$TMP" -O /dev/null "$URL"
fi
rc=$?
rm -f "$TMP"
[ "$rc" -eq 0 ] || logger -t xc-report "上报失败，退出码 $rc"
exit "$rc"
