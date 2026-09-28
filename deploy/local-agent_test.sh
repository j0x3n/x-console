#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
cd "$test_dir"
. "$script_dir/local-agent.sh"

mock_log="$test_dir/docker.log"
mock_config="$test_dir/agent-config"
mock_active="$test_dir/agent-active"
mock_fail="$test_dir/fail-install"
mock_fail_enable="$test_dir/fail-enable"

docker() {
	printf '%s\n' "$*" >>"$mock_log"
	if [ "$1" = compose ]; then
		case "$2" in
			cp) : >./x-console-agent ;;
			exec) printf 'TEST-CODE\n' ;;
		esac
		return 0
	fi
	case " $* " in
		*'/host/etc/x-console-agent/config.json'*) [ -f "$mock_config" ]; return $? ;;
		*'/src:ro'*) [ ! -f "$mock_fail" ]; return $? ;;
	esac
	while [ "$1" != -- ]; do shift; done
	shift
	case "$1:$2" in
		/usr/local/bin/x-console-agent:pair) : >"$mock_config" ;;
		systemctl:is-active) [ -f "$mock_active" ]; return $? ;;
		systemctl:enable) [ ! -f "$mock_fail_enable" ] || return 1; : >"$mock_active" ;;
		systemctl:disable) rm -f "$mock_active" ;;
		systemctl:restart) [ -f "$mock_active" ]; return $? ;;
		rm:-rf) rm -f "$mock_config" ;;
	esac
}

printf 'XC_LOCAL_AGENT=1\nXC_PORT=17400\n' >.env
manage_local_agent >/dev/null
[ -f .local-agent-installed ] && [ -f "$mock_config" ] && [ -f "$mock_active" ]
grep -q 'nsenter -t 1 -m -u -n -i -r --' "$mock_log"
grep -q -- '--server http://127.0.0.1:17400 --code TEST-CODE' "$mock_log"

: >"$mock_log"
manage_local_agent >/dev/null
grep -q 'systemctl restart x-console-agent' "$mock_log"
if grep -q 'pairing-code' "$mock_log"; then echo '重复配对' >&2; exit 1; fi

rm -f "$mock_active"
: >"$mock_log"
manage_local_agent >/dev/null
if grep -q 'systemctl restart\|pairing-code' "$mock_log"; then echo '吊销后重新启动' >&2; exit 1; fi

printf 'XC_LOCAL_AGENT=0\n' >.env
manage_local_agent >/dev/null
[ ! -f .local-agent-installed ] && [ ! -f "$mock_config" ] && [ ! -f ./x-console-agent ]

printf 'XC_LOCAL_AGENT=1\n' >.env
: >"$mock_fail"
if manage_local_agent >/dev/null 2>&1; then echo '安装失败未返回错误' >&2; exit 1; fi
[ ! -f .local-agent-installed ]
rm -f "$mock_fail"
manage_local_agent >/dev/null
[ -f .local-agent-installed ] && [ -f "$mock_config" ]

printf 'XC_LOCAL_AGENT=0\n' >.env
manage_local_agent >/dev/null
printf 'XC_LOCAL_AGENT=1\n' >.env
: >"$mock_fail_enable"
if manage_local_agent >/dev/null 2>&1; then echo '启动失败未返回错误' >&2; exit 1; fi
[ -f .local-agent-pending ] && [ -f "$mock_config" ] && [ ! -f .local-agent-installed ]
rm -f "$mock_fail_enable"
: >"$mock_log"
manage_local_agent >/dev/null
[ -f .local-agent-installed ] && [ ! -f .local-agent-pending ] && [ -f "$mock_active" ]
if grep -q 'pairing-code' "$mock_log"; then echo '安装重试时重复配对' >&2; exit 1; fi
echo '本机代理安装、更新、吊销、卸载测试通过'
