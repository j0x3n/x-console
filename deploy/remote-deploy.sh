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
	} >.env
	echo "已创建 .env。请立即备份其中的 XC_MASTER_KEY：$(pwd)/.env"
fi

# 记下这次部署的镜像版本，docker compose 从 .env 读取 XC_IMAGE。
if grep -q '^XC_IMAGE=' .env; then
	sed -i "s|^XC_IMAGE=.*|XC_IMAGE=$IMAGE|" .env
else
	echo "XC_IMAGE=$IMAGE" >>.env
fi

docker compose pull x-console caddy
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
echo "部署完成：$(docker compose exec -T x-console wget -qO- http://127.0.0.1:8080/api/v1/health)"
docker image prune -f >/dev/null
