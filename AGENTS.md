# AGENTS.md

X Console 是一个人用的控制台：管服务器和 Windows 本机、跑编码任务、项目看板、笔记、云盘、提醒、习惯、日历、Home Assistant、GitHub。后端 Go + SQLite，前端 React + TypeScript，部署在用户自己的服务器上。

## 先读什么

1. `docs/tasks.md`：任务看板，当前进度和下一步只看这里。
2. 要做的任务对应的规格：`docs/specs/`。
3. 写代码前读相关的约定：后端 `docs/03-backend.md`，前端 `docs/05-frontend.md`，代理 `docs/04-agent-protocol.md`。其他文档见 `docs/README.md`。
4. **做任何界面之前必读 `docs/07-design.md`**：页面结构、组件清单、具体数值、交活前自查。先复制最像的现有页面再改，用现成的公共组件，不要自己发明样式。

## 用户偏好（所有角色都要遵守）

- 回复一律用中文，像中文母语者口语写出来的，不是翻译腔。
- 一句话一个意思。不用破折号插入语。不写超过两个逗号的长句。
- 不用比喻、类比、拟人。只用日常词和行业标准术语。
- 界面文案同样遵守这几条。
- 用户额度有限：先确认方向再动手，少做无关的探索。

## 分工

- **开发者**（目前是 GPT）：按 `docs/tasks.md` 的顺序做任务，提 PR。
- **审查者**（目前是 Claude）：用户提新需求时写成任务和规格，不写功能代码；用户让验收时审查 PR。只有用户明确要求时才写功能代码。
- **用户**：决定做什么，合并 PR，决定何时上线。

用户让你做什么，你就是哪个角色。

## 流程

1. **记需求**（审查者）：在 `docs/tasks.md` 的“待做”末尾加一条，编号顺延（B10、B11……）。大任务在 `docs/specs/` 写规格：要什么、怎么做、验收标准。提交到 `develop`。
2. **开发**（开发者）：
   - 从 `develop` 开分支 `task/<编号>-<英文短名>`，比如 `task/B2-overview`。
   - 在 `docs/tasks.md` 把任务移到“进行中”，写上负责人。
   - 开发，跑完下面的全部检查。有界面改动时再跑 `npm run shots`，按 `docs/07-design.md` 第七节自查截图。
   - 提 PR 到 `develop`，按 PR 模板写清楚改了什么。CI 会自动跑测试。
3. **验收**（审查者）：
   - 跑全部检查。起真实服务端和代理，跑 `npm run shots` 出 1360px 和 390px 的截图，看有没有报错和横向溢出，并对照 `docs/07-design.md` 看样式。
   - 重点看安全：高危操作是否调用 `auth.RequireElevated`，令牌是否加密存储，代理执行命令的限制，公开入口是否校验密钥。
   - 在 PR 上写评论，问题按严重程度排序。没问题就说可以合并。
4. **合并**：用户合并 PR。开发者在 `docs/tasks.md` 把任务移到“已完成”。
5. **上线**：用户同意后，把 `develop` 合并进 `main`。推送到 `main` 会自动部署到线上服务器。

## 构建和部署（2026-09-27 用户要求）

- 平时推送到 `develop` 不跑任何构建，省构建额度。推送前自己在本地跑完下面的全部检查。
- 一批改动做完再推送，不要每个小提交推一次。
- **用户说“构建”或“部署”时**：在要推送的最后一个提交信息里加 `[deploy]`（比如标题末尾写 ` [deploy]`），推送到 `develop`。这会跑测试、构建镜像，并部署到线上服务器，让用户在线上看效果。不需要再问，也不需要合并到 `main`。
- 用户没说时，提交信息里不要带 `[deploy]`。
- 推送到 `main`、在 Actions 页面手动运行 Deploy，也会部署。PR 上会自动跑检查。

## 常用命令

```bash
# 一次性安装代码生成工具
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0

# 后端检查（提交前全部要过）
cd backend
PATH=$PATH:~/go/bin go generate ./...   # 生成代码，结果要提交
gofmt -l .                              # 不能有输出
go vet ./...
go test -race ./...
GOOS=windows GOARCH=amd64 go build ./cmd/agent
GOOS=windows GOARCH=amd64 go vet ./internal/agent/... ./cmd/agent

# 前端检查
cd web
npm ci
npm run gen:api                         # 从 api/modules/*.yaml 生成类型，结果要提交
npm run typecheck && npm test && npm run build

# 本地运行
cd backend && export XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1 && go run ./cmd/server   # 127.0.0.1:8080
cd web && npm run dev                                                                                # 127.0.0.1:5173
```

CI 会检查生成的代码是否最新：生成之后 `git diff` 必须为空。

## 写代码的规则

- 只改自己任务涉及的模块目录：`backend/internal/server/modules/<模块>`、`web/src/features/<模块>`、`api/modules/<模块>.yaml`、对应的迁移和规格。
- 共享文件只加行，不改别人的行：`backend/internal/server/app/modules.go`、`backend/sqlc.yaml`、`backend/cmd/agent/main.go`、`web/src/features/settings/tabs.tsx`、`web/src/app/TopbarActions.tsx`、`web/src/app/GlobalPanels.tsx`。
- 基础代码尽量不动：`backend/internal/server/` 下除 `modules/` 以外的目录、`backend/pkg/`、`web/src/{api,auth,app,components,lib}`。必须改时，改动要小、要向后兼容，在 PR 里写理由，在 `docs/tasks.md` 的“接口变更记录”里记一笔。
- 迁移文件名用 `YYYYMMDDHHMMSS_m<编号>_<说明>.sql`，时间要晚于已有的最新迁移。
- 模块集成测试放外部测试包（`package xxx_test`），需要内部函数时用 `export_test.go` 暴露。否则 testutil → app → 模块会循环引用。
- `db/models.go` 是生成文件，合并冲突时取任一边，再 `go generate ./...`。
- 事务直接用 `db.BeginTx`，连接已设 `_txlock=immediate`。
- 用 coder/websocket 写数据时，不要传会被取消的 context，它会关掉整条连接。
- 不要对 `web/src/api/gen/` 运行 prettier。
- 中文词典是全局的：同一个英文键在所有模块里必须对应同一个中文，意思不同就换一个英文键。`web/src/lib/i18n.test.ts` 会检查。

## Git 规则

- `main` 是线上版本，推送或合并到它会自动部署。只有用户同意才能动它。
- `develop` 上最后一个提交带 `[deploy]` 的推送也会部署到线上，只在用户说“构建”“部署”时这样做（见上面的“构建和部署”）。
- `develop` 是开发分支，任务分支都从它开，PR 都提到它。
- 不要强推，不要改写已推送的历史。
