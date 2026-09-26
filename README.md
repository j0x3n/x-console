# X Console

一个人用的控制台。管服务器和 Windows 本机，在本机仓库上跑编码任务（Claude Code / Codex），管项目、备忘、提醒和习惯，接 Home Assistant、GitHub、Linear 和 Claude。

## 组成

- `web/`：前端，React 19 + TypeScript + Vite。
- `backend/`：Go。`cmd/server` 是服务端，`cmd/agent` 是装在服务器和本机上的代理。
- `api/`：接口契约（OpenAPI），前后端代码都从这里生成。
- `deploy/`：Docker、Caddy、systemd、Windows 安装脚本。
- `docs/`：开发文档。从 [docs/README.md](docs/README.md) 开始读。

## 快速开始

```bash
# 后端
cd backend
export XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1
go run ./cmd/server

# 前端（另一个终端）
cd web && npm ci && npm run dev
```

打开 http://127.0.0.1:5173，按提示创建账号并绑定两步验证。

部署到服务器见 [docs/06-deploy.md](docs/06-deploy.md)。

## 开发进度

见 [docs/roadmap.md](docs/roadmap.md)。
