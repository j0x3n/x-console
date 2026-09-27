# 架构

## 已定的技术决定

这些是和用户确认过的，不要推翻。要改先问用户。

| 事项 | 结论 |
| --- | --- |
| 部署 | 面板和 API 在一台服务器上（Docker）。Windows 本机和各台 Linux 装代理，代理主动连服务器 |
| 后端 | Go 1.26，SQLite（modernc.org/sqlite，纯 Go），chi，goose 迁移，sqlc，oapi-codegen |
| 前端 | React 19 + TypeScript（严格）+ Vite + React Router 8 + TanStack Query + Zustand |
| 接口契约 | `api/modules/<模块>.yaml`（OpenAPI 3.0），前后端代码都从它生成 |
| 编码执行器 | Claude Code 和 Codex CLI 都支持 |
| 项目管理 | 自建为主，可选同步 Linear |
| 登录 | 单用户，密码登录，两步验证（TOTP）在设置里可选；高危操作要求 5 分钟内再验证一次（验证码或密码）。2026-09-27 用户调整 |
| 服务器 | 1 到 10 台 Linux，amd64 |
| 反向代理 | 用户服务器上已有 Caddy。面板容器只监听 `127.0.0.1:17380`，由用户的 Caddy 反代 |
| 镜像 | 只构建 linux/amd64 |

## 组件

```
浏览器 / 手机 PWA ──HTTPS──> Caddy ──> x-console-server (Go + SQLite)
                                          │
              ┌─── WebSocket（代理主动连）──┤── 出站 HTTPS：Telegram、Bark、Server酱、
              │                            │   Home Assistant、GitHub、Linear、Claude API
   x-console-agent (Windows 本机)     x-console-agent (各台 Linux)
```

| 组件 | 职责 |
| --- | --- |
| web | 界面。只和 server 通信 |
| x-console-server | 登录、数据、定时任务、通知投递、第三方集成、给代理下指令 |
| x-console-agent | 在它所在的机器上执行：采指标、管进程和服务、终端、传文件、跑编码任务 |
| Caddy | HTTPS 证书和反向代理 |

## 为什么代理主动连

服务器在公网，你的电脑在家里的内网。服务器连不进来。代理从内网主动连出去，建一条 WebSocket 长连接。服务端通过这条连接下发请求。

一条连接上同时跑三种东西：

- 请求和响应：比如“列出进程”。
- 流：终端、跟随日志、编码任务输出。每个流有自己的 id。
- 事件：代理主动上报，比如每 10 秒一次的指标。

协议细节见 [04-agent-protocol.md](04-agent-protocol.md)。

## Windows 代理怎么跑

用任务计划程序，在你登录 Windows 时启动。不要装成 Windows 服务。
原因：Windows 服务跑在 session 0，拿不到你的剪贴板、git 凭据和 Claude Code 的登录状态。

## 数据

- 数据库是一个 SQLite 文件，开 WAL 模式。
- 时间一律存 UTC，列类型写 `DATETIME`，由 Go 写入。
- 用户所在时区由 `XC_TZ` 决定，默认 `Asia/Shanghai`。“今天”“每天 8 点”这类计算都用这个时区。
- 第三方令牌、SSH 私钥、TOTP 密钥用 AES-256-GCM 加密后入库。主密钥在环境变量 `XC_MASTER_KEY`，不进数据库。
- 指标：10 秒一条的原始数据只在内存里保留 1 小时。1 分钟粒度存 7 天。1 小时粒度存 90 天。

## 安全

| 风险 | 做法 |
| --- | --- |
| 密码被猜 | argon2id 存储；同一 IP 15 分钟内失败 5 次就锁住 |
| 密码泄露 | 登录必须带 TOTP |
| 会话被偷后做坏事 | 高危操作要求 5 分钟内重新验证 TOTP（`auth.RequireElevated`） |
| CSRF | Cookie 设 SameSite=Strict；所有写请求必须带 `X-Requested-With: x-console` |
| 跨站 WebSocket | `websocket.Accept` 默认校验 Origin |
| 代理令牌泄露 | 每台一个令牌，库里只存哈希，设置页可以一键吊销 |
| 配对码被截获 | 一次性，10 分钟过期，生成时要二次验证 |
| 事后追查 | 审计日志记录每一次写操作和远程执行 |
| Webhook 伪造 | 每个公开入口自己校验密钥（比如 Telegram 的 secret_token） |

高危操作清单（必须调用 `auth.RequireElevated`）：

- 生成配对码、吊销代理
- 打开终端、执行脚本、执行任意命令
- 结束进程、停止服务、关机
- 删除文件、上传文件到服务器
- 编码任务的提交、推送、丢弃
- 修改第三方集成的令牌
- 删除自动化规则里带执行动作的规则

## 事件

服务端内部有一个事件总线（`events.Bus`）。模块做了改动就发布事件，主题格式是 `<实体>.<动作>`，例如 `issue.updated`、`host.metrics`、`reminder.due`。

事件有三个用途：

1. 推给浏览器。前端据此刷新数据，不需要轮询。
2. 自动化引擎（M12）拿它当触发器。
3. 模块之间解耦。比如编码任务完成后，项目模块订阅 `coding_task.finished` 更新 Issue。

## 模块之间怎么调用

- 同步调用：模块在 `New` 里用 `module.Provide` 注册一个接口，别的模块在 `Start` 里用 `module.Lookup` 取出来。
- 异步通知：发布事件。
- 不要直接 import 另一个模块的内部包。只能 import 对方公开的接口类型包，比如 `modules/habits/habitsapi`。

## 部署形态

单个 Go 二进制加一个 SQLite 文件，前端构建产物由 Go 直接托管（`XC_WEB_DIR`），前面放 Caddy。见 [06-deploy.md](06-deploy.md)。
