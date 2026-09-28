#!/bin/sh
set -eu
cd "$(dirname "$0")"
. ./local-agent.sh

set_image() {
	if grep -q '^XC_IMAGE=' .env; then
		sed -i "s|^XC_IMAGE=.*|XC_IMAGE=$1|" .env
	else
		printf 'XC_IMAGE=%s\n' "$1" >>.env
	fi
}

fetch_image() {
	docker image inspect "$1" >/dev/null 2>&1 || docker pull -q "$1" >/dev/null
}

health() {
	i=0
	until docker compose exec -T x-console wget -qO- http://127.0.0.1:8080/api/v1/health >/dev/null 2>&1; do
		i=$((i + 1))
		if [ "$i" -ge 30 ]; then
			return 1
		fi
		sleep 2
	done
}

restore_database() {
	backup=$1
	case "$backup" in
		x-console-*.db) ;;
		*) echo "无效的备份文件名：$backup" >&2; return 1 ;;
	esac
	case "$backup" in
		*/*|*'..'*) echo "无效的备份文件名：$backup" >&2; return 1 ;;
	esac
	docker compose run --rm --no-deps -T --entrypoint sh x-console -c 'test -f "$1"' sh "/data/backups/$backup"
	docker compose stop x-console
	docker compose run --rm --no-deps -T --user 0 --entrypoint sh x-console -c '
		set -e
		test -f "$1"
		cp "$1" /data/x-console.db
		chown 10001:10001 /data/x-console.db
		rm -f /data/x-console.db-wal /data/x-console.db-shm
	' sh "/data/backups/$backup"
}

rollback_to() {
	image=$1
	backup=$2
	set_image "$image"
	if restore_database "$backup" && docker compose up -d --no-build --remove-orphans && health; then
		echo "已回退到 $image，数据恢复自 $backup"
		return 0
	fi
	echo "回退失败，需要人工检查容器和备份：$backup" >&2
	return 1
}

read_previous() {
	if [ ! -f .previous-deploy ]; then
		echo "没有可回退的上一版本" >&2
		return 1
	fi
	OLD_IMAGE=$(sed -n '1p' .previous-deploy)
	BACKUP=$(sed -n '2p' .previous-deploy)
	if [ -z "$OLD_IMAGE" ] || [ -z "$BACKUP" ]; then
		echo "上一版本记录不完整" >&2
		return 1
	fi
}

case "${1:-}" in
	rollback)
		[ "$#" -eq 1 ] || { echo "用法: remote-deploy.sh rollback" >&2; exit 2; }
		read_previous
		rollback_to "$OLD_IMAGE" "$BACKUP"
		exit
		;;
	restore)
		[ "$#" -eq 2 ] || { echo "用法: remote-deploy.sh restore <备份文件>" >&2; exit 2; }
		[ -f .env ] || { echo "找不到 .env" >&2; exit 1; }
		restore_database "$2"
		docker compose up -d --no-build --remove-orphans
		health
		echo "已恢复数据库：$2"
		exit
		;;
	'') echo "用法: remote-deploy.sh <镜像>|rollback|restore <备份文件>" >&2; exit 2 ;;
esac
IMAGE=$1

if [ ! -f .env ]; then
	if [ -z "${XC_DOMAIN:-}" ]; then
		echo "没有 .env，也没有提供 XC_DOMAIN。" >&2
		exit 1
	fi
	fetch_image "$IMAGE"
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

OLD_IMAGE=$(sed -n 's/^XC_IMAGE=//p' .env)
BACKUP=
CURRENT_CONTAINER=$(docker compose ps -a -q x-console)
RUNNING=$(docker compose ps --status running -q x-console)
if [ -n "$RUNNING" ]; then
	[ -n "$OLD_IMAGE" ] || { echo "运行中的容器缺少 XC_IMAGE，无法记录旧版本" >&2; exit 1; }
	TAG=${OLD_IMAGE##*:}
	TAG=$(printf '%s' "$TAG" | tr -c '[:alnum:]._-' '_')
	BACKUP="x-console-$(date -u +%Y%m%d-%H%M%S)-$TAG.db"
	docker compose exec -T x-console sh -c 'mkdir -p /data/backups && chmod 700 /data/backups'
	if ! docker compose exec -T x-console x-console-server backup "/data/backups/$BACKUP"; then
	set_image "$IMAGE"
	if ! fetch_image "$IMAGE" || ! docker compose run --rm --no-deps -T x-console backup "/data/backups/$BACKUP"; then
		set_image "$OLD_IMAGE"
		exit 1
	fi
	set_image "$OLD_IMAGE"
fi
	docker compose exec -T x-console chmod 600 "/data/backups/$BACKUP"
	echo "部署前数据库备份：$BACKUP"
elif [ -n "$CURRENT_CONTAINER" ]; then
	echo "旧容器没有运行，无法生成部署前备份" >&2
	exit 1
fi

set_image "$IMAGE"
if ! fetch_image "$IMAGE" || ! docker compose up -d --no-build --remove-orphans || ! health; then
	echo "新版本没有就绪，最近日志：" >&2
	docker compose logs --tail 50 x-console >&2 || true
	if [ -n "$BACKUP" ]; then
		rollback_to "$OLD_IMAGE" "$BACKUP" || exit 1
	fi
	exit 1
fi

if [ -n "$BACKUP" ]; then
	printf '%s\n%s\n' "$OLD_IMAGE" "$BACKUP" >.previous-deploy
	docker compose exec -T x-console sh -c 'ls -1t /data/backups/x-console-*.db | tail -n +11 | xargs -r rm --'
fi
if ! manage_local_agent; then
	echo "警告：本机代理安装或更新失败，面板部署已完成" >&2
fi
PORT=$(sed -n 's/^XC_PORT=//p' .env)
echo "部署完成，面板在 127.0.0.1:${PORT:-17380}：$(docker compose exec -T x-console wget -qO- http://127.0.0.1:8080/api/v1/health)"
