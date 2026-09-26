#!/bin/sh
# 在服务器上运行，由 GitHub Actions 通过 SSH 调用。也可以手动运行：
#   ./remote-deploy.sh ghcr.io/j0x3n/x-console:latest
# 第一次运行且没有 .env 时，需要环境变量 XC_DOMAIN，脚本会生成 .env 和主密钥。
set -eu
cd "$(dirname "$0")"
IMAGE="${1:?用法: remote-deploy.sh <镜像>}"

if [ ! -f .env ]; then
	if [ -z "${XC_DOMAIN:-}" ]; then
		echo "没有 .env，也没有提供 XC_DOMAIN。请设置 GitHub Secret DEPLOY_DOMAIN，或手动创建 .env。" >&2
		exit 1
	fi
	docker pull -q "$IMAGE" >/dev/null
	KEY=$(docker run --rm "$IMAGE" gen-key)
	umask 077
	{
		echo "XC_DOMAIN=$XC_DOMAIN"
		echo "XC_PUBLIC_URL=https://$XC_DOMAIN"
		echo "XC_MASTER_KEY=$KEY"
		echo "XC_TZ=${XC_TZ:-Asia/Shanghai}"
		echo "XC_PORT=${XC_PORT:-17380}"
		echo "XC_LOCAL_AGENT=1"
	} >.env
	echo "已创建 .env。请立即备份其中的 XC_MASTER_KEY：$(pwd)/.env"
fi

# 记下这次部署的镜像版本，docker compose 从 .env 读取 XC_IMAGE。
if grep -q '^XC_IMAGE=' .env; then
	sed -i "s|^XC_IMAGE=.*|XC_IMAGE=$IMAGE|" .env
else
	echo "XC_IMAGE=$IMAGE" >>.env
fi

docker compose pull
docker compose up -d --remove-orphans

# 等服务健康。
i=0
until docker compose exec -T x-console wget -qO- http://127.0.0.1:8080/api/v1/health >/dev/null 2>&1; do
	i=$((i + 1))
	if [ "$i" -ge 30 ]; then
		echo "服务 60 秒内没有就绪，最近日志：" >&2
		docker compose logs --tail 50 x-console >&2
		exit 1
	fi
	sleep 2
done
PORT=$(grep '^XC_PORT=' .env | cut -d= -f2)
echo "部署完成，面板在 127.0.0.1:${PORT:-17380}：$(docker compose exec -T x-console wget -qO- http://127.0.0.1:8080/api/v1/health)"

# docker 组用户通过临时特权容器在主机上执行 systemctl 和代理命令。
host_exec() {
	docker run --rm --privileged --pid=host --network=host alpine:3.22 sh -c \
		'apk add --no-cache util-linux >/dev/null && exec nsenter -t 1 -m -u -n -i -r -w -- "$@"' sh "$@"
}

install_agent_files() {
	docker run --rm -v /:/host -v "$PWD:/src:ro" alpine:3.22 sh -c '
		install -m 755 /src/x-console-agent /host/usr/local/bin/x-console-agent.new &&
		mv -f /host/usr/local/bin/x-console-agent.new /host/usr/local/bin/x-console-agent &&
		install -m 644 /src/x-console-agent.service /host/etc/systemd/system/x-console-agent.service'
}

remove_local_agent() {
	if [ ! -f .local-agent-installed ]; then
		return 0
	fi
	host_exec sh -c '
		systemctl disable --now x-console-agent || true
		rm -f /usr/local/bin/x-console-agent /etc/systemd/system/x-console-agent.service
		rm -rf /etc/x-console-agent
		systemctl daemon-reload' || return 1
	rm -f .local-agent-installed
}

manage_local_agent() (
	trap 'rm -f ./x-console-agent' EXIT
	local_agent=$(sed -n 's/^XC_LOCAL_AGENT=//p' .env | tail -n 1)
	if [ "${local_agent:-1}" = 0 ]; then
		remove_local_agent
		return $?
	fi
	if [ "${local_agent:-1}" != 1 ]; then
		echo "XC_LOCAL_AGENT 只能是 0 或 1" >&2
		return 1
	fi
	if [ ! -f .local-agent-installed ] && ! host_exec test -f /etc/x-console-agent/config.json; then
		docker compose cp x-console:/usr/local/bin/x-console-agent ./x-console-agent || return 1
		code=$(docker compose exec -T x-console x-console-server pairing-code --name "$(hostname)" --kind server) || return 1
		install_agent_files || return 1
		host_exec /usr/local/bin/x-console-agent pair --server "http://127.0.0.1:${PORT:-17380}" \
			--code "$code" --config /etc/x-console-agent/config.json || return 1
		host_exec systemctl daemon-reload || return 1
		host_exec systemctl enable --now x-console-agent || return 1
		touch .local-agent-installed || return 1
	elif host_exec systemctl is-active --quiet x-console-agent; then
		docker compose cp x-console:/usr/local/bin/x-console-agent ./x-console-agent || return 1
		install_agent_files || return 1
		host_exec systemctl daemon-reload || return 1
		host_exec systemctl restart x-console-agent || return 1
	fi
)

if ! manage_local_agent; then
	echo "警告：本机代理安装或更新失败。面板已部署成功。" >&2
fi

# 只保留本次和上一次的 x-console 镜像（方便回滚），不动服务器上的其他镜像。
REPO="${IMAGE%:*}"
docker images "$REPO" --format '{{.Repository}}:{{.Tag}}' |
	grep -v -x -F "$IMAGE" | grep -v ':<none>$' | tail -n +2 |
	xargs -r docker rmi >/dev/null 2>&1 || true
docker image prune -f --filter "label=org.opencontainers.image.source" >/dev/null 2>&1 || true
