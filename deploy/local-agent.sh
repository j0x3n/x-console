# Installed on the deployment host, not inside the panel container.
host_run() {
	docker run --rm --privileged --pid=host alpine:3.22 \
		nsenter -t 1 -m -u -n -i -r -- "$@"
}

host_has_config() {
	docker run --rm -v /:/host:ro alpine:3.22 \
		test -f /host/etc/x-console-agent/config.json
}

install_host_agent_files() {
	docker compose cp x-console:/usr/local/bin/x-console-agent ./x-console-agent || return 1
	docker run --rm -v /:/host -v "$PWD:/src:ro" alpine:3.22 sh -c '
		set -e
		install -m 755 /src/x-console-agent /host/usr/local/bin/x-console-agent.new
		mv -f /host/usr/local/bin/x-console-agent.new /host/usr/local/bin/x-console-agent
		install -m 644 /src/x-console-agent.service /host/etc/systemd/system/x-console-agent.service
	' || return 1
}

uninstall_local_agent() {
	if [ ! -f .local-agent-installed ] && [ ! -f .local-agent-pending ] && ! host_has_config; then return 0; fi
	host_run systemctl disable --now x-console-agent >/dev/null 2>&1 || true
	host_run rm -f /usr/local/bin/x-console-agent /etc/systemd/system/x-console-agent.service || return 1
	host_run rm -rf /etc/x-console-agent || return 1
	host_run systemctl daemon-reload || return 1
	rm -f .local-agent-installed .local-agent-pending ./x-console-agent
	echo "已卸载面板所在服务器的代理"
}

manage_local_agent() {
	LOCAL_AGENT=$(sed -n 's/^XC_LOCAL_AGENT=//p' .env | tail -n 1)
	case "${LOCAL_AGENT:-1}" in
		0) uninstall_local_agent; return $? ;;
		1) ;;
		*) echo "XC_LOCAL_AGENT 只能是 0 或 1" >&2; return 1 ;;
	esac
	AGENT_PORT=$(sed -n 's/^XC_PORT=//p' .env | tail -n 1)
	AGENT_PORT=${AGENT_PORT:-17380}
	if [ -f .local-agent-pending ] && host_has_config; then
		install_host_agent_files || return 1
		host_run systemctl daemon-reload || return 1
		host_run systemctl enable --now x-console-agent || return 1
		host_run systemctl is-active --quiet x-console-agent || return 1
		touch .local-agent-installed
		rm -f .local-agent-pending
		echo "已完成面板所在服务器的代理安装"
		return 0
	fi
	if ! host_has_config && [ ! -f .local-agent-installed ]; then
		touch .local-agent-pending
		install_host_agent_files || return 1
		PAIR_CODE=$(docker compose exec -T x-console x-console-server pairing-code --name "$(hostname)" --kind server) || return 1
		host_run /usr/local/bin/x-console-agent pair \
			--server "http://127.0.0.1:$AGENT_PORT" --code "$PAIR_CODE" \
			--config /etc/x-console-agent/config.json || return 1
		host_run systemctl daemon-reload || return 1
		host_run systemctl enable --now x-console-agent || return 1
		host_run systemctl is-active --quiet x-console-agent || return 1
		touch .local-agent-installed
		rm -f .local-agent-pending
		echo "已安装面板所在服务器的代理"
	elif host_run systemctl is-active --quiet x-console-agent; then
		install_host_agent_files || return 1
		host_run systemctl daemon-reload || return 1
		host_run systemctl restart x-console-agent || return 1
		echo "已更新面板所在服务器的代理"
	fi
}
