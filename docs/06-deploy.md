# 部署与本地联调

## 服务端：GitHub Actions 自动部署（推荐）

`.github/workflows/deploy.yml` 会依次做：跑测试，构建 amd64 镜像推到 `ghcr.io/j0x3n/x-console`，构建代理程序（Linux 和 Windows），然后 SSH 到服务器更新。

测试和构建都在 GitHub 的机器上跑，不占你服务器的资源。服务器每次部署只拉镜像、重启容器，前后几十秒。

### 1. 准备服务器（只做一次）

1. 装 Docker 和 compose 插件：`curl -fsSL https://get.docker.com | sh`。
2. 建一个部署用户，加进 docker 组，让它不用 sudo 就能跑 docker：
   ```bash
   sudo adduser --disabled-password deploy
   sudo usermod -aG docker deploy
   ```
3. 在你自己的电脑上生成一对专用密钥，不要设密码：
   ```bash
   ssh-keygen -t ed25519 -f xconsole_deploy -C github-actions -N ""
   ```
   把 `xconsole_deploy.pub` 的内容追加到服务器的 `/home/deploy/.ssh/authorized_keys`。
4. 域名加一条 A 记录指向服务器 IP。
5. 配好反向代理，见下一节。

### 反向代理

面板容器只监听服务器本机的 `127.0.0.1:17380`，外网直接访问不到。HTTPS 由反向代理负责。端口可以在服务器的 `~/x-console/.env` 里用 `XC_PORT` 改。

**服务器上已经有 Caddy（直接装在系统里）**：在你的 Caddyfile 里加一段，然后 `sudo systemctl reload caddy`：

```
console.example.com {
	reverse_proxy 127.0.0.1:17380
}
```

WebSocket（终端、实时推送）不用额外配置，Caddy 会自动处理。

**Caddy 本身跑在 Docker 里**：容器里的 `127.0.0.1` 指的是 Caddy 容器自己，连不到面板。改成 `reverse_proxy host.docker.internal:17380`，并在 Caddy 容器上加 `extra_hosts: ["host.docker.internal:host-gateway"]`。这种情况下，还要把面板的端口映射从 `127.0.0.1:17380` 改成 `172.17.0.1:17380`，或者让两个容器加入同一个 Docker 网络，再反代到 `x-console:8080`。

**服务器上没有反向代理**：在 `~/x-console/.env` 里加一行 `COMPOSE_PROFILES=caddy`，再部署一次，就会启动自带的 Caddy。它会自动申请证书，要求 80 和 443 端口空闲并在防火墙放行。

用 Nginx 也可以，注意转发 WebSocket 的 `Upgrade` 和 `Connection` 头，并设置 `X-Forwarded-For`。

### 2. 在 GitHub 填 Secrets

仓库页面 → Settings → Secrets and variables → Actions → New repository secret。

| 名称 | 必填 | 内容 |
| --- | --- | --- |
| `DEPLOY_HOST` | 是 | 服务器 IP 或域名 |
| `DEPLOY_USER` | 是 | 部署用户，比如 `deploy` |
| `DEPLOY_SSH_KEY` | 是 | 私钥 `xconsole_deploy` 的全部内容，包括首尾两行 |
| `DEPLOY_DOMAIN` | 首次部署必填 | 面板域名，比如 `console.example.com` |
| `DEPLOY_PORT` | 否 | SSH 端口，默认 22 |
| `DEPLOY_PATH` | 否 | 部署目录，默认部署用户家目录下的 `x-console` |
| `DEPLOY_KNOWN_HOSTS` | 否，建议填 | 服务器指纹，在你电脑上运行 `ssh-keyscan -p 22 服务器IP` 得到。不填时首次连接自动信任 |

没填 `DEPLOY_HOST` 时工作流只构建不部署，不会报错。

### 3. 触发部署

只有两种情况会部署：

- 推送到 `main`，通常是把 `develop` 合并进 `main`。
- 在 Actions 页面选 Deploy，点 Run workflow 手动运行。

其他分支的推送和 PR 只跑测试，不构建镜像，也不部署。

回滚：在 Actions 页面找到上一次成功的 Deploy，点 Re-run all jobs。

### 服务器资源保护

- 只保留当前和上一个版本的镜像，方便回滚。不会删服务器上其他程序的镜像。
- 每个容器的日志最多 3 个 10 MB 文件，自动轮换。
- 面板容器内存上限 512 MB。
- 服务器指标数据按保留期自动清理（原始数据 1 小时，分钟级 7 天，小时级 90 天）。

第一次部署时，服务器上没有 `.env`，脚本会自动生成一份，里面有随机生成的主密钥 `XC_MASTER_KEY`。**请马上登录服务器备份这个文件**：`~/x-console/.env`。丢了主密钥，存进去的令牌就解不开了。

部署完打开 `https://你的域名`，按提示创建账号并绑定两步验证。

自动部署会在面板所在服务器上安装代理。这台服务器随后会出现在“服务器”页面。代理连接本机的面板端口，不经过公网。首次安装需要主机使用 systemd。安装失败只会在部署日志里显示警告，不影响面板启动。

代理程序在每次运行的 Artifacts 里下载：`x-console-agent-linux-amd64`、`x-console-agent-windows-amd64`。

### 镜像是私有的

私有仓库推到 ghcr.io 的镜像默认也是私有的。自动部署时工作流会临时登录拉取镜像，不用你管。要在服务器上手动拉取，先用一个有 `read:packages` 权限的 GitHub 令牌 `docker login ghcr.io`。

## 服务端：手动部署

不用 GitHub Actions 时，在服务器上：

```bash
git clone https://github.com/j0x3n/x-console.git && cd x-console/deploy
cp .env.example .env
# 填 XC_DOMAIN、XC_PUBLIC_URL，生成主密钥：
echo "XC_MASTER_KEY=$(openssl rand -base64 32)" >> .env
echo "XC_IMAGE=x-console:local" >> .env
docker compose up -d --build
```

### 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `XC_MASTER_KEY` | 无，必填 | 32 字节的 base64，用来加密令牌 |
| `XC_ADDR` | `127.0.0.1:8080` | 监听地址。Docker 里是 `0.0.0.0:8080` |
| `XC_DATA_DIR` | `./data` | 数据库目录。Docker 里是 `/data` |
| `XC_WEB_DIR` | 空 | 前端构建产物目录。空表示只提供 API |
| `XC_PUBLIC_URL` | 空 | 外部访问地址，通知里的链接用它拼 |
| `XC_TZ` | `Asia/Shanghai` | 你的时区 |
| `XC_DEV` | 空 | 设为 `1` 时 Cookie 不带 Secure，只用于本地 HTTP 调试 |
| `XC_DEBUG` | 空 | 设为 `1` 输出调试日志 |
| `XC_IMAGE` | `ghcr.io/j0x3n/x-console:latest` | 仅 docker compose 使用，自动部署会写入具体版本 |
| `XC_PORT` | `17380` | 仅 docker compose 使用，面板在服务器本机监听的端口 |
| `XC_LOCAL_AGENT` | `1` | 自动安装并更新本机代理。设为 `0` 后再部署一次，会卸载自动安装的代理 |
| `COMPOSE_PROFILES` | 空 | 设为 `caddy` 时启动自带的 Caddy |

### 备份

数据库在 Docker 卷 `xc-data` 里。在部署目录下运行：

```bash
docker compose exec -T x-console sh -c 'cat /data/x-console.db' > backup-$(date +%F).db
```

服务运行时这样复制，极少数情况下会拿到写了一半的文件。更稳妥的做法是先停服务：`docker compose stop x-console`，复制完再 `docker compose start x-console`。同时备份 `.env`。

## 代理

### Linux 服务器

自动部署的面板主机无需手动安装。其他 Linux 服务器可按下面的步骤配对。

```bash
# 编译（在开发机上）
cd backend && GOOS=linux GOARCH=amd64 go build -o x-console-agent ./cmd/agent
# 复制到服务器后
sudo install x-console-agent /usr/local/bin/
sudo x-console-agent pair --server https://console.example.com --code <配对码> --config /etc/x-console-agent/config.json
sudo cp deploy/x-console-agent.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now x-console-agent
```

在“设置 → 设备与代理”里吊销面板主机的代理后，代理会停止。后续部署不会重新配对它。要彻底卸载，在服务器部署目录的 `.env` 里设置 `XC_LOCAL_AGENT=0`，再部署一次。要重新加入，先用 `0` 部署完成卸载，再改回 `1` 部署一次。

### Windows 本机

```powershell
# 编译
cd backend; $env:GOOS="windows"; go build -o x-console-agent.exe ./cmd/agent
# 安装（普通 PowerShell 即可）
.\deploy\install-agent-windows.ps1 -Server https://console.example.com -Code <配对码> -Binary .\backend\x-console-agent.exe
```

## 本地联调

```bash
# 终端 1：后端
cd backend
export XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1
go run ./cmd/server

# 终端 2：前端
cd web && npm run dev
# 打开 http://127.0.0.1:5173

# 终端 3：本机代理（在设置页生成配对码后）
cd backend
go run ./cmd/agent pair --server http://127.0.0.1:8080 --code <配对码> --config ./data/agent.json
go run ./cmd/agent run --config ./data/agent.json
```

Windows 上用 PowerShell 设置环境变量：`$env:XC_MASTER_KEY = go run ./cmd/server gen-key; $env:XC_DEV = "1"`。
