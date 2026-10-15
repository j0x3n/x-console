#!/bin/sh
# X Console 路由器上报脚本（B114）。由 cron 每分钟启动一次。
# 它把路由器的状态拼成一份纯文本，POST 给面板。面板不需要访问路由器。
# 面板的回复里有两样东西：下次上报的间隔（interval=秒），以及要执行的命令（cmd=编号 动作 参数）。
# 间隔小于 60 秒时，脚本在这一分钟里按间隔循环上报，到分钟末尾退出，由 cron 接着启动。
URL='@URL@'
TOKEN='@TOKEN@'
# 临时文件放哪里。测试时用 XC_REPORT_DIR 换掉
DIR=${XC_REPORT_DIR:-/tmp}
TMP=$DIR/xc-report.body
RESP=$DIR/xc-report.resp
LOCK=$DIR/xc-report.pid

# 上一次启动的还在跑就不重复启动
if [ -f "$LOCK" ] && kill -0 "$(cat "$LOCK" 2>/dev/null)" 2>/dev/null; then
	exit 0
fi
echo $$ > "$LOCK"
trap 'rm -f "$LOCK" "$TMP" "$RESP"' EXIT

# Wi-Fi 上连着的终端（B228）。每个接口一段：
#   =hostapd <ubus 对象名>，后面是 ubus call 的 JSON（clients 的键是 MAC）
#   =iwinfo <接口>，后面是 iwinfo <接口> assoclist 的原文
# 有 hostapd 的 ubus 对象就用它，没有再用 iwinfo。解析放在面板里
collect_wifi() {
	found=0
	for obj in $(ubus list 'hostapd.*' 2>/dev/null); do
		echo "=hostapd $obj"
		ubus call "$obj" get_clients 2>/dev/null
		found=1
	done
	[ "$found" -eq 1 ] && return
	for d in "${XC_SYS_NET:-/sys/class/net}"/*; do
		[ -d "$d/wireless" ] || [ -d "$d/phy80211" ] || continue
		i=${d##*/}
		echo "=iwinfo $i"
		iwinfo "$i" assoclist 2>/dev/null
	done
}

collect() {
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
		echo '#xc:wifi'
		collect_wifi
		# 没有 ip 命令就不带这一段，面板退回读上面的 arp
		if command -v ip >/dev/null 2>&1; then
			echo '#xc:neigh'
			ip neigh show 2>/dev/null
		fi
		# nlbwmon（带宽监控）里每台设备的字节数（B229）。没装就不带这一段
		if command -v nlbw >/dev/null 2>&1; then
			echo '#xc:nlbw'
			nlbw -c json -g mac 2>/dev/null
		fi
	} > "$TMP"
}

send() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsS -m 20 -H "Authorization: Bearer $TOKEN" -H 'Content-Type: text/plain' \
			--data-binary @"$TMP" -o "$RESP" "$URL"
	else
		uclient-fetch -q -T 20 --header="Authorization: Bearer $TOKEN" --header='Content-Type: text/plain' \
			--post-file="$TMP" -O "$RESP" "$URL"
	fi
}

# 只执行这两种命令，接口名只许字母、数字和 _ . -
run_command() {
	case "$2" in
	restart_interface)
		case "$3" in
		'' | *[!A-Za-z0-9_.-]*) return ;;
		esac
		(
			ubus call "network.interface.$3" down
			ubus call "network.interface.$3" up
		) > /dev/null 2>&1 &
		;;
	reboot)
		(
			sleep 2
			reboot
		) > /dev/null 2>&1 &
		;;
	esac
}

INTERVAL=60
START=$(date +%s)
COUNT=0
while :; do
	collect || exit 1
	send
	rc=$?
	if [ "$rc" -ne 0 ]; then
		logger -t xc-report "上报失败，退出码 $rc"
		exit "$rc"
	fi
	while IFS= read -r line; do
		case "$line" in
		interval=*)
			v=${line#interval=}
			case "$v" in
			'' | *[!0-9]*) ;;
			*) if [ "$v" -ge 3 ] && [ "$v" -le 60 ]; then INTERVAL=$v; fi ;;
			esac
			;;
		cmd=*)
			# shellcheck disable=SC2086
			set -- ${line#cmd=}
			run_command "$@"
			;;
		esac
	done < "$RESP"
	COUNT=$((COUNT + 1))
	NOW=$(date +%s)
	# 这一分钟里放不下下一次，就退出，由 cron 接着启动
	if [ $((NOW - START + INTERVAL)) -gt 57 ] || [ "$COUNT" -ge 25 ]; then
		break
	fi
	sleep "$INTERVAL"
done
