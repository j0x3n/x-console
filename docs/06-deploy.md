# 部署与本地联调

## 服务端：GitHub Actions 自动部署（推荐）

`.github/workflows/deploy.yml` 会同时跑测试和构建 amd64 镜像（推到 `ghcr.io/j0x3n/x-console`，先只带 `sha-<提交号>` 标签）。两个都成功后，SSH 到服务器更新，并把这个镜像标成 `latest`。测试没过时不会上线，`latest` 也不变。代理程序（Linux 和 Windows）在测试通过后构建。整个流程大约 3 分多钟。

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

以下三种情况会构建并部署：

- 推送到 `main`，通常是把 `develop` 合并进 `main`。
- 推送到 `develop`，且最后一个提交的信息里带 `[deploy]`。用户说“构建”“部署”时这样做，用来在线上看开发中的效果。
- 在 Actions 页面选 Deploy，点 Run workflow 手动运行，可以选分支。

平时推送到 `develop` 不跑任何构建。PR 上会跑测试（只改文档的 PR 不跑），不构建镜像，也不部署。

回滚：在 Actions 页面找到上一次成功的 Deploy，点 Re-run all jobs。

### 服务器资源保护

- 只保留当前和上一个版本的镜像，方便回滚。不会删服务器上其他程序的镜像。
- 每个容器的日志最多 3 个 10 MB 文件，自动轮换。
- 面板容器内存上限 512 MB。
- 服务器指标数据按保留期自动清理（原始数据 1 小时，分钟级 7 天，小时级 90 天）。

第一次部署时，服务器上没有 `.env`，脚本会自动生成一份，里面有随机生成的主密钥 `XC_MASTER_KEY`。**请马上登录服务器备份这个文件**：`~/x-console/.env`。丢了主密钥，存进去的令牌就解不开了。

部署完打开 `https://你的域名`，按提示创建账号并绑定两步验证。

代理程序在每次运行的 Artifacts 里下载：`x-console-agent-linux-amd64`、`x-console-agent-windows-amd64`。

自动部署会把同版本的代理装到面板所在的 Linux 服务器。安装成功后，这台机器会自动出现在“服务器”页面。代理使用本机地址连接面板，不经过公网域名。部署用户需在 docker 组，主机需使用 systemd。安装失败只会在部署日志里警告，不会撤回已经就绪的面板。

在设置 → 设备与代理里吊销这台机器后，代理会停止，后续部署不会把它重新加回来。要彻底卸载，在服务器部署目录的 `.env` 中设 `XC_LOCAL_AGENT=0`，再部署一次。此操作会停止服务，并删除代理程序、systemd 单元和配置。想重新加入，先设为 `0` 部署一次，再设为 `1` 部署一次。

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
| `XC_PUBLIC_URL` | 空 | 外部访问地址，通知里的链接用它拼，一键安装脚本里的面板地址也用它。不设时用请求里的 `Host` 和 `X-Forwarded-Proto` |
| `XC_AGENTS_DIR` | `/usr/share/x-console/agents` | 面板发给别的机器安装的代理程序。Docker 镜像里已经打包，本地开发没有这个目录，下载接口回 404 |
| `XC_TZ` | `Asia/Shanghai` | 你的时区 |
| `XC_DEV` | 空 | 设为 `1` 时 Cookie 不带 Secure，只用于本地 HTTP 调试 |
| `XC_DEBUG` | 空 | 设为 `1` 输出调试日志 |
| `XC_IMAGE` | `ghcr.io/j0x3n/x-console:latest` | 仅 docker compose 使用，自动部署会写入具体版本 |
| `XC_PORT` | `17380` | 仅 docker compose 使用，面板在服务器本机监听的端口 |
| `XC_LOCAL_AGENT` | `1` | 自动安装并更新面板所在服务器的代理；设为 `0` 并部署一次可卸载 |
| `COMPOSE_PROFILES` | 空 | 设为 `caddy` 时启动自带的 Caddy |

### 备份与回退

每次部署前，脚本通过 SQLite 的 `VACUUM INTO` 生成一致的数据库副本。备份位于 `xc-data` 卷的 `/data/backups/`，文件名包含 UTC 时间和旧镜像版本，保留最近 10 份。首次部署没有旧容器时跳过备份。新版本 60 秒内未通过健康检查时，脚本会停止新容器，恢复数据库和旧镜像，并检查旧服务是否就绪。本次部署仍以失败退出。

在服务器的部署目录里回退最近一次成功部署：

```bash
sh remote-deploy.sh rollback
```

只恢复数据库并重启当前镜像：

```bash
docker compose exec -T x-console ls -1 /data/backups/
sh remote-deploy.sh restore x-console-20260927-153000-sha-8fed186.db
```

恢复数据库会丢失该备份之后写入的数据。请单独保存 `.env`，以免丢失解密令牌所需的主密钥。

### 整站备份和恢复（B25）

面板的“设置 → 备份”可以导出整站：一个 `.tar.gz`，里面有数据库的一致副本、全部文件和 `manifest.json`。导出的包放在 `/data/backups/`，和部署脚本的 `.db` 备份放在同一个目录，互不影响。部署脚本只清理 `x-console-*.db`。

- 自动备份可以每天或每周一次，传到 S3 的 `backups/` 目录。用存储设置里的 S3 时，包在存储的前缀下面，搬迁存储位置时不会被当成站点文件搬走。
- 令牌和密钥在包里仍是加密的。恢复到另一台机器时，要用同一个 `XC_MASTER_KEY`，否则恢复后这些设置要重新填写，页面会列出是哪几项。

恢复分两步：

1. 在面板里点“恢复”。服务先备份当前数据（恢复前备份），再把包解到 `/data/restore-tmp/`，然后停止进程。
2. 进程被 Docker 重新拉起（`restart: unless-stopped`）。启动时先把新数据库换到位，再打开数据库并升级，最后把包里的文件写进文件存储，多出来的旧文件会删掉。

所以恢复要求服务由 Docker、systemd 这类工具自动重启。手动运行的进程停止后，自己再启动一次就会接着完成恢复。恢复完成后所有登录状态会以备份里的为准，需要重新登录。

恢复前备份保存在同一个目录，恢复出了问题可以用它再恢复一次。

## 代理

### 一条命令添加（推荐）

在设置 → 设备与代理里点“添加设备”，填名称、选类型，面板生成配对码和命令。配对码 10 分钟内有效，只能用一次。

- Linux 服务器：`curl -fsSL "<面板>/api/v1/agent/install.sh?code=XXXX-XXXX" | sudo sh`。脚本会判断 amd64 或 arm64，从面板下载代理并校验，装到 `/usr/local/bin/x-console-agent`，配对，写 systemd 服务（限制内存 128 MB、CPU 20%）并启动。没有 systemd 时装好程序和配对，打印手动启动命令。
- Windows 电脑：PowerShell 里执行 `irm "<面板>/api/v1/agent/install.ps1?code=XXXX-XXXX" | iex`，或者下载 `<面板>/api/v1/agent/setup.exe?code=XXXX-XXXX` 双击。代理装到 `%LOCALAPPDATA%\x-console-agent`，用任务计划程序在登录时启动。
- 已经装过的机器再执行一次命令只升级程序，不会再配对，也不会多出一条记录。要重新配对，在 `sh` 后面加 `-s -- --repair`（Windows 删掉 `%APPDATA%\x-console-agent\config.json` 再执行）。升级时也要在面板里生成一个新的配对码，用过的码会得到 404。
- 卸载：`curl -fsSL "<面板>/api/v1/agent/uninstall.sh" | sudo sh`。面板里的记录要在设备页里吊销。
- 代理是主动连面板的，内网机器只要能访问面板地址就行，不用开端口。
- 资源实测（2026-09-30）：在限制为 1 核、1 GB 的 Linux 容器中，服务端和代理同容器运行。代理连接后预热 10 秒，再取 30 秒的进程 CPU 时间差和末尾 RSS。空闲时 RSS 11.1 MiB、平均 CPU 0.00%；通过 `/events` 订阅详情并要求 1 秒刷新时，RSS 13.8 MiB、平均 CPU 0.10%。CPU 读数精度约 0.03 个百分点。真实服务器上的 systemd 安装仍需验收。
- 这些接口不用登录，但要带有效的配对码，无效、过期或用过的码一律回 404，同一个 IP 每分钟最多 10 次请求，超过回 429。脚本里的面板地址来自 `XC_PUBLIC_URL`，没设时来自请求的 `Host`，只接受字母、数字、`.`、`-`、`:` 组成的地址。
- 代理程序在镜像里，四个平台各一份，在 `/usr/share/x-console/agents/<系统>-<架构>/`，旁边有 `SHA256SUMS`。下载接口的响应头 `X-Checksum-Sha256` 是文件的校验值，脚本用它检查。

### 手动安装

没法用一键脚本时（比如机器访问不了面板地址，只能自己拷文件），按下面的步骤。

#### Linux 服务器

```bash
# 编译（在开发机上）
cd backend && GOOS=linux GOARCH=amd64 go build -o x-console-agent ./cmd/agent
# 复制到服务器后
sudo install x-console-agent /usr/local/bin/
sudo x-console-agent pair --server https://console.example.com --code <配对码> --config /etc/x-console-agent/config.json
sudo cp deploy/x-console-agent.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now x-console-agent
```

#### Windows 电脑

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
