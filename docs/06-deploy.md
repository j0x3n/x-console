# 部署与本地联调

## 服务端（Docker）

在你的服务器上：

```bash
git clone https://github.com/j0x3n/x-console.git && cd x-console/deploy
cp .env.example .env
# 填 XC_DOMAIN、XC_PUBLIC_URL，生成主密钥：
echo "XC_MASTER_KEY=$(openssl rand -base64 32)" >> .env
docker compose up -d --build
```

打开 `https://你的域名`，按页面提示创建账号并绑定 TOTP。

主密钥一定要备份。丢了它，库里加密的令牌就解不开了。

### 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `XC_MASTER_KEY` | 无，必填 | 32 字节的 base64，用来加密令牌 |
| `XC_ADDR` | `127.0.0.1:8080` | 监听地址。Docker 里是 `0.0.0.0:8080` |
| `XC_DATA_DIR` | `./data` | 数据库目录 |
| `XC_WEB_DIR` | 空 | 前端构建产物目录。空表示只提供 API |
| `XC_PUBLIC_URL` | 空 | 外部访问地址，通知里的链接用它拼 |
| `XC_TZ` | `Asia/Shanghai` | 你的时区 |
| `XC_DEV` | 空 | 设为 `1` 时 Cookie 不带 Secure，只用于本地 HTTP 调试 |
| `XC_DEBUG` | 空 | 设为 `1` 输出调试日志 |

### 备份

数据都在 `deploy/data/x-console.db`。用 SQLite 的在线备份：

```bash
sqlite3 deploy/data/x-console.db ".backup '/backup/x-console-$(date +%F).db'"
```

## 代理

### Linux 服务器

```bash
# 编译（在开发机上）
cd backend && GOOS=linux GOARCH=amd64 go build -o x-console-agent ./cmd/agent
# 复制到服务器后
sudo install x-console-agent /usr/local/bin/
sudo x-console-agent pair --server https://console.example.com --code <配对码> --config /etc/x-console-agent/config.json
sudo cp deploy/x-console-agent.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now x-console-agent
```

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
