# X Console 交接文档

更新时间：2026-09-27
开发分支：`claude/focused-wozniak-i2fda9`（远端 `origin` 已同步）
仓库：https://github.com/j0x3n/x-console

这份文档给接手开发的人或 AI 看。读完它，再读 `docs/README.md`，就能接着干。

---

## 1. 这是什么

一个人用的控制台，用户只有一个人（仓库主人）。部署在用户自己的服务器上，电脑和手机都能打开。功能包括：

- 管理多台 Linux 服务器和一台 Windows 电脑（指标、进程、服务、Web 终端、文件、Docker）
- 在 Windows 电脑的本地仓库里跑编码任务（Claude Code 或 Codex CLI），看 diff、提交、建 PR
- 项目看板和 Issue（类似 Linear），可选和 Linear 双向同步
- 备忘、提醒、习惯打卡（喝水、健身）、日历、每日早报、番茄钟
- 通知推送到站内、Web Push、Telegram、Bark、Server酱
- 接 Home Assistant、GitHub
- AI 助手（Claude API tool use）和自动化规则

完整的功能清单和验收标准在 `docs/01-product.md`。

## 2. 用户的偏好（必须遵守）

- **回答一律用中文**。要像中文母语者口语写出来的，不是翻译腔。
- 一句话一个意思。不用破折号插入语。不写超过两个逗号的长句。
- 不用比喻、类比、拟人。解释抽象概念就举具体场景。
- 只用日常词和行业标准术语。不用“赋能、沉淀、抓手”这类词。
- 界面文案同样遵守上面几条。
- 用户额度有限，偏好：先确认方向，再大段开发；每批做完合并、验证、推送。

## 3. 已确认的技术决定（不要推翻）

| 事项 | 结论 |
| --- | --- |
| 部署 | 面板和 API 在一台服务器上（Docker）。Windows 本机和各台 Linux 装代理，代理主动连服务器 |
| 后端 | Go 1.26，SQLite（modernc.org/sqlite，纯 Go），chi，goose 迁移，sqlc，oapi-codegen |
| 前端 | React 19 + TypeScript（严格）+ Vite + React Router 8 + TanStack Query + Zustand |
| 接口契约 | `api/modules/<模块>.yaml`（OpenAPI 3.0），前后端代码都从它生成 |
| 编码执行器 | Claude Code 和 Codex CLI 都支持 |
| 项目管理 | 自建为主，可选同步 Linear |
| 登录 | 单用户，密码 + TOTP；高危操作要求 5 分钟内再验证一次 TOTP |
| 服务器 | 1 到 10 台 Linux，amd64 |
| 反向代理 | 用户服务器上已有 Caddy。面板容器只监听 `127.0.0.1:17380`，由用户的 Caddy 反代 |
| 镜像 | 只构建 linux/amd64 |

## 4. 当前进度

### 开发批次（详见 `docs/roadmap.md`）

| 批次 | 内容 | 状态 |
| --- | --- | --- |
| 0 | M0 基础、前端外壳、协议、contracts、actions、全部文档 | 完成 |
| 1 | M5 项目、M6 备忘、M7 提醒通知、M8 习惯、M2/M3 服务器和本机、M9 Home Assistant | 完成，已合并，浏览器验收通过 |
| 2 | M13 GitHub + Linear | 完成，已合并 |
| 2 | M4 编码任务、M10 运维监控、M11 日历早报番茄钟 | 见第 12 节“最后状态” |
| 3 | M12 AI 助手与自动化；M1 首页、命令面板增强、PWA、端到端测试 | **未开始**，下一步做这个 |

### 已有的后端模块

`backend/internal/server/modules/` 下：`hosts`（M2/M3）、`projects`（M5）、`notes`（M6）、`reminders`（M7）、`habits`（M8）、`homeassistant`（M9）、`github` 和 `linear`（M13）。批次 2 剩下的模块合并后会多出编码任务、运维监控、日历等目录。

### 前端

`web/src/features/` 下每个模块一个目录。还是占位页（显示“即将推出”）的：`overview`（首页，M1）、`automations` 和 `assistant`（M12）。批次 2 未合并的模块也是占位页。

## 5. 仓库结构

```
api/                    接口契约。common.yaml 共用结构；modules/<模块>.yaml
backend/                Go，一个模块 github.com/j0x3n/x-console/backend
  cmd/server            服务端入口（x-console-server，gen-key 子命令生成主密钥）
  cmd/agent             代理入口（pair / run / version），register() 和 capabilities() 接各功能包
  internal/server/
    app/                组装；modules.go 是模块列表
    module/             模块接口 Deps、Module、Starter、PublicPather、Registry
    auth audit secrets settings events scheduler notify agenthub ws httpx
    actions/            动作目录（给 AI 助手和自动化用）
    contracts/          跨模块接口
    core/               M0 的接口实现
    store/migrations/   所有迁移
    testutil/           测试用完整服务器
    modules/<模块>/     功能模块
  internal/agent/       代理端各功能包
  pkg/protocol          服务端和代理之间的消息定义
  pkg/rpc               一条 WebSocket 上的请求、流、事件
  sqlc.yaml             每个模块一项
web/                    前端
deploy/                 Dockerfile、docker-compose.yml、Caddyfile、remote-deploy.sh、systemd、Windows 安装脚本
docs/                   开发文档
.github/workflows/      ci.yml（测试）、deploy.yml（测试 → 镜像 → 代理程序 → 部署）
```

## 6. 必读文档

| 文档 | 内容 |
| --- | --- |
| `docs/03-backend.md` | 新增模块的 9 个步骤、处理器写法、基础服务用法、动作目录、测试要求 |
| `docs/04-agent-protocol.md` | 代理配对、连接、请求/流/事件、新增方法的步骤 |
| `docs/05-frontend.md` | 模块文件结构、调接口、组件和样式、全局入口 |
| `docs/modules/Mx.md` | 每个模块的规格。已完成的模块已按实际实现更新 |
| `docs/roadmap.md` | 批次、任务卡、变更记录、批次 1 的经验 |

## 7. 常用命令

```bash
# 一次性：安装代码生成工具
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0

# 后端验证（提交前全部要过）
cd backend
PATH=$PATH:~/go/bin go generate ./...
gofmt -l .                      # 必须没有输出
go vet ./...
go test -race ./...
GOOS=windows GOARCH=amd64 go build ./cmd/agent
GOOS=windows GOARCH=amd64 go vet ./internal/agent/... ./cmd/agent

# 前端验证
cd web
npm ci
npm run gen:api                 # 从 api/modules/*.yaml 生成 src/api/gen/*.ts，结果要提交
npm run typecheck && npm test && npm run build

# 本地运行
cd backend
export XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1
go run ./cmd/server                                   # 127.0.0.1:8080
cd web && npm run dev                                 # 127.0.0.1:5173，/api 转发到 8080
# 代理：设置页生成配对码后
go run ./cmd/agent pair --server http://127.0.0.1:8080 --code <配对码> --config ./data/agent.json
go run ./cmd/agent run --config ./data/agent.json
```

CI 会检查生成的代码是否最新（`go generate` 和 `npm run gen:api` 之后 `git diff` 必须为空）。

## 8. 部署现状

- **已经上线**，用户在自己的服务器上能打开面板。面板容器监听 `127.0.0.1:17380`，由用户已有的 Caddy 反代并提供 HTTPS。
- 流水线是 `.github/workflows/deploy.yml`：测试 → 构建镜像推到 `ghcr.io/j0x3n/x-console` → 构建 Linux 和 Windows 代理程序（在 Artifacts 里下载）→ SSH 到服务器运行 `deploy/remote-deploy.sh`。
- **只有这三种情况会部署**：推送到 `main`；提交信息里带 `[deploy]`；手动运行（手动按钮要等工作流文件进入默认分支 `main` 才会出现）。其他推送只跑测试，不构建镜像。
- 所以线上版本不会自动跟着开发分支更新。要更新线上，推一个提交信息带 `[deploy]` 的提交。更新线上前先问用户。
- 服务器上的部署目录默认是部署用户家目录下的 `x-console`。`.env` 里有主密钥 `XC_MASTER_KEY`，丢了就解不开存进去的令牌。
- 服务器资源保护：只保留当前和上一个镜像；容器日志轮换（3 个 10 MB）；面板内存上限 512 MB。
- 详细说明在 `docs/06-deploy.md`。
- 开发分支还没合并进 `main`。是否合并、何时合并，先问用户。

## 9. 下一步：批次 3

任务卡在 `docs/roadmap.md`，规格在 `docs/modules/M12.md`、`docs/modules/M1.md`、`docs/01-product.md` 的“全局”部分。

### I：M12 AI 助手与自动化

- 用官方 Go SDK `github.com/anthropics/anthropic-sdk-go`。默认模型 `claude-opus-5`，adaptive thinking，流式输出，开启服务端 refusal fallback（`fallbacks: "default"` 加 beta 头 `server-side-fallback-2026-07-01`）。写代码前查官方 Go SDK 文档确认用法，不要凭记忆。
- 工具来自 `d.Actions.List()`。各模块已经注册了动作（`grep -rn "Actions.Register" backend/internal/server/modules`）。
- `read` 动作直接执行；`write` 等用户确认；`dangerous` 要确认并要求提升权限。
- 自动化：触发器（定时、事件、指标、HA 状态、Webhook）+ 条件 + 动作。HA 实体要调用 `contracts.HomeAssistant.WatchEntity`。
- 前端：`features/assistant`、`features/automations`；在 `app/GlobalPanels.tsx` 加侧边面板，在 `app/TopbarActions.tsx` 加按钮。
- 批次 2 的 M11 早报里预留了“AI 润色”的位置，M12 完成后接上。

### J：M1 首页与收尾

- `features/overview`：卡片网格，数据来自各模块已有的 hooks；每张卡片包错误边界；布局存服务端。
- 命令面板支持前缀输入：`> 内容` 直接存成笔记（现在 `components/command/CommandPalette.tsx` 不支持把输入的文字传给命令）。
- PWA：manifest、图标、安装提示。`web/public/sw.js` 已有推送处理，文件顶部标了缓存逻辑的接入点。
- 路由懒加载，解决构建时主包超过 500 kB 的警告。
- 所有页面 390px 宽度检查。
- Playwright 端到端测试加进 CI。

## 10. 合并流程和踩过的坑

之前的做法：每个任务卡在独立 git worktree 的分支上开发，完成后由负责人合并到开发分支。合并时：

- 共享文件（`app/modules.go`、`sqlc.yaml`、`cmd/agent/main.go`、`features/settings/tabs.tsx`、`go.mod`、`go.sum`）的冲突都是“双方各加了几行”，两边都保留，按模块编号排序。
- `core/db/models.go` 和各模块的 `db/models.go` 是 sqlc 生成的，冲突时取任一边，再运行 `go generate ./...` 重新生成。
- 合并后运行 `go mod tidy`，跑第 7 节的全部验证。

踩过的坑：

1. **worktree 可能从旧的 `main` 创建**，没有任何新代码。开工前确认分支包含开发分支的最新提交。
2. **迁移文件名的时间戳**必须晚于 M0 的 `20260927000000`，否则会排在 M0 前面执行。
3. **集成测试放在外部测试包**（`package xxx_test`），需要内部函数时用 `export_test.go` 暴露。否则 testutil → app → 模块会循环引用。
4. **不要对 `web/src/api/gen/` 运行 prettier**（已有 `.prettierignore`）。
5. **事务直接用 `db.BeginTx`**。连接已设 `_txlock=immediate`，不要自己写 `BEGIN IMMEDIATE`。
6. **coder/websocket 会在写入的 context 被取消时关闭整条连接**。`pkg/rpc` 已处理，别的地方直接用这个库写 WebSocket 时也要注意。
7. **浏览器验证**：用 Playwright。脚本里 `spawn` 出来的子进程要在结束时杀掉，否则 node 不退出。

## 11. 已知问题和待办

### 没有在真实环境验证过

- Windows 代理的运行时行为：ConPTY 终端、Windows 服务管理、剪贴板、锁屏关机、打开程序。只在 Linux 上交叉编译和静态检查过。
- 真实的 Claude Code 和 Codex CLI（测试用的是假执行器）。
- 真实的 Home Assistant、Telegram、Bark、Server酱、Web Push、GitHub、Linear（测试全部用假服务器）。
- systemd 服务管理（开发环境没有 systemd，只有解析逻辑的测试）。
- 真实的 SSH 主机（只用进程内的假 SSH 服务器测过）。

### 已知限制

- 同一个 TOTP 码在 30 秒窗口内可以重复使用。
- 只支持一个用户。
- HA 的 `WatchEntity` 注册只存在内存里，使用方要在 `Start` 里调用。
- GitHub 每个仓库只拉第一页（100 个 PR）。
- Linear 不导入已完成或已取消的 Issue；本地新建的 Issue 不会自动建到 Linear。
- 看板拖动只支持桌面，手机上在详情页改状态。
- 习惯的提醒时段不能跨午夜。
- SSH 主机的最后在线时间只存在内存里。
- Windows 上 `svc.logs` 返回“不支持”（没读 Windows 事件日志）。

### 可以改进

- 文件上传进度条。
- SSH 主机指纹变化后，在界面上重新信任。
- `features/reminders` 和 `features/habits` 的 `api.ts` 在修改后自己刷新数据，同时也用了 `invalidateOn`，有重复，可以精简。
- `features/projects` 的 Markdown 渲染器（`mdparse.ts`、`Markdown.tsx`）可以挪到 `components/ui` 给其他模块用。

## 12. 最后状态

（批次 2 剩余任务合并后在这里更新。）
