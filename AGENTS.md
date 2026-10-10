# AGENTS.md

X Console 是一个人用的控制台：管服务器和 Windows 本机、跑编码任务、项目看板、笔记、云盘、提醒、习惯、日历、Home Assistant、GitHub。后端 Go + SQLite，前端 React + TypeScript，部署在用户自己的服务器上。

## 先读什么

1. `docs/tasks.md`：任务看板，当前进度和下一步只看这里。有 `docs/handoff.md` 时先读它，那是正在做的一批任务的交接状态。
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

- **开发者**：按 `docs/tasks.md` 的顺序做任务。平时直接在 `develop` 上开发（2026-10-02 用户定的）。
- **Codex**：用户让 Codex 做某一批时，Codex 在单独的 `codex` 分支上做，做完开 PR 给 Claude 审查。
- **审查者**（目前是 Claude）：用户提新需求时写成任务和规格；用户让验收时，审查 `codex` 分支上还没合并的提交。用户让写代码时直接在 `develop` 上写。
- **用户**：决定做什么，一批做完后合并一次，决定何时上线。

用户让你做什么，你就是哪个角色。

## 流程

1. **记需求**（审查者）：在 `docs/tasks.md` 的“待做”末尾加一条，编号顺延（B10、B11……）。大任务在 `docs/specs/` 写规格：要什么、怎么做、验收标准。提交到 `develop`。
2. **开发**（2026-10-02 用户定的做法）：
   - `develop` 是唯一的开发分支，平时直接在上面提交、推送。不能直接推送到 `main`。
   - 只有用户让 Codex 做的时候才用 `codex` 分支：从最新的 `develop` 开出 `codex`（已经有就先把 `develop` 合并进来，用 merge，不要 rebase），这一批都提交到 `codex`。
   - 在 `docs/tasks.md` 把任务移到“进行中”，负责人写清楚是谁。
   - **一个任务一个提交**，提交信息以任务编号开头，比如 `B11: 笔记附件后端`。一个任务里改了几次，推送前合成一个提交。这样哪个任务有问题，`git revert` 那一个提交就能单独撤掉。
   - 截图不提交（仓库是公开的，2026-10-10 用户定的）。`npm run shots` 出的图只在本地看，检查结果写进 `docs/qa/` 的文字记录。
   - 做完一个任务，跑完下面的全部检查再推送。有界面改动时再跑 `npm run shots`，按 `docs/07-design.md` 第七节自查截图。检查没过不要推送。
   - Codex 第一次推送后开一个 PR：`codex` → `develop`，标题“Codex 开发批次”。以后一直往 `codex` 推，这个 PR 会自动更新，CI 每次都会跑。不要每个任务开一个 PR。
   - 任务做完从 `docs/tasks.md` 删掉，在 `docs/tasks-done.md` 的“已完成”表加一行，写上提交号。`tasks.md` 只放没做完的事，保持短。
3. **验收**（审查者，按批次）：用户说“验收”时，看 `develop..codex` 之间的全部提交：
   - 跑全部检查。起真实服务端和代理，跑 `npm run shots` 出 1360px 和 390px 的截图，看有没有报错和横向溢出，并对照 `docs/07-design.md` 看样式。
   - 逐个任务看：有没有改自己模块以外的文件，有没有测试，和规格对不对得上。
   - 重点看安全：高危操作是否调用 `auth.RequireElevated`，令牌是否加密存储，代理执行命令的限制，公开入口是否校验密钥。
   - 在那个 PR 上写一条评论：每个任务“通过 / 要改”，问题按严重程度排序。不通过的任务，开发者在 `codex` 上改，或者 `git revert` 那个任务的提交。
4. **合并**：全部通过后，用户在 PR 页面用 **“Create a merge commit”** 合并（不要用 Squash，保留每个任务的提交，方便以后单独撤）。合并后删掉 `codex` 分支，下次 Codex 再做时从 `develop` 重新开。
5. **出了问题怎么退**：
   - 某个任务不要了：在它所在的分支上 `git revert <那个任务的提交>`。
   - Codex 整批都不要了：关掉 PR，删掉 `codex`（这一步要用户同意）。
   - 想回到交给 Codex 之前的样子：分支 `frontend-done` 就是那时的版本。
6. **上线**：用户同意后，把 `develop` 合并进 `main`。推送到 `main` 会自动部署到线上服务器。

## 构建和部署（2026-09-27 用户要求）

- 平时推送到 `develop`、`codex` 不跑部署，省构建额度。`codex` 有开着的 PR，推送时会跑 CI 检查；直接推到 `develop` 没有 CI。推送前自己在本地跑完下面的全部检查。
- 一批改动做完再推送，不要每个小提交推一次。
- **用户说“构建”或“部署”时**：在要推送的最后一个提交信息里加 `[deploy]`（比如标题末尾写 ` [deploy]`），推送到你正在用的分支（一般是 `develop`，Codex 是 `codex`）。这会跑测试、构建镜像，并部署到线上服务器，让用户在线上看效果。不需要再问，也不需要合并到 `main`。
- 从 `codex` 部署时，线上跑的是还没验收的代码。部署前会自动备份数据库（B19 做完以后），出了问题可以退回。
- 用户没说时，提交信息里不要带 `[deploy]`。标题和正文都算，写这条规则本身时也要写成“部署标记”，不要写出这几个字符。
- 推送到 `main`、在 Actions 页面手动运行 Deploy，也会部署。PR 上会自动跑检查。

## 保持稳定（2026-09-27 用户要求）

项目会越来越大，下面几条每个任务都要做到：

- 按“规格 → 接口定义 → 测试 → 实现”的顺序做。缺规格先写规格，缺测试不算做完。
- 做功能任务（B、C 编号的任务），要跑全部检查，不能只跑这个模块的测试。小修按下面“小修”一节。
- B8 做完以后，每个新功能都要在端到端测试里补一条主流程。
- 数据库迁移只加不删。删列、改列分两次部署：先停止使用，下个版本再删。
- 每做完大约 5 个功能任务，排一轮清理（任务看板里的 C1、C2……），这一轮不加新功能。
- 发现的问题当场修不了，记到 `docs/tasks.md` 的“已知问题”，不要放着不说。

## 小修（2026-09-30 用户要求）

用户直接让你改的小问题（界面错位、文案、一个接口的小 bug），不要跑全量检查。光是全量后端测试一次就要 3 分半左右（2026-10-01 在 Linux 容器里测的，带 -race，含第一次编译）。部署时的后端测试不带 -race。再加上前端构建、截图、端到端测试更久。这些 CI 会替你跑。

- 什么算小修：不加迁移，不改 `api/modules/*.yaml`，改动集中在一两个模块。超出这个范围按功能任务做，先告诉用户。
- 本地只跑 `scripts/check-quick.sh`：按改动的文件只跑相关的生成、格式、编译、测试和类型检查。缓存热的时候几十秒到几分钟。
- 不跑 `npm run shots`、`npm run e2e`、全量 `go test -race ./...`、`npm run build`。改了界面只截改动的那一页：`npm run shots -- --only /notes`。
- 完整检查交给 CI：Codex 推送到 `codex` 时 PR 的 CI 会跑；带部署标记的推送会先跑全部测试，过了才部署。直接推到 `develop` 又不带部署标记时没有 CI，下次部署时才会跑到。CI 红了再修。
- 先定位再动手：先读报错和相关代码，找到原因再改。不要为了“确认没问题”去跑和这次改动无关的测试。
- 超过 10 分钟还没改好，停下来告诉用户卡在哪、打算怎么办。
- 做完用两三句话汇报：改了什么、跑了哪些检查。

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
npm test && npm run build                # build 里已经包含 typecheck

# 小修只跑这个（见“小修”一节）
scripts/check-quick.sh

# 本地运行
cd backend && export XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1 && go run ./cmd/server   # 127.0.0.1:8080
cd web && npm run dev                                                                                # 127.0.0.1:5173
```

CI 会检查生成的代码是否最新：生成之后 `git diff` 必须为空。

## 写代码的规则

- 只改自己任务涉及的模块目录：`backend/internal/server/modules/<模块>`、`web/src/features/<模块>`、`api/modules/<模块>.yaml`、对应的迁移和规格。
- 共享文件只加行，不改别人的行：`backend/internal/server/app/modules.go`、`backend/sqlc.yaml`、`backend/cmd/agent/main.go`、`web/src/features/settings/tabs.tsx`、`web/src/app/TopbarActions.tsx`、`web/src/app/GlobalPanels.tsx`。
- 基础代码尽量不动：`backend/internal/server/` 下除 `modules/` 以外的目录、`backend/pkg/`、`web/src/{api,auth,app,components,lib}`。必须改时，改动要小、要向后兼容，在 PR 里写理由，在 `docs/api-changes.md` 里记一笔。
- 迁移文件名用 `YYYYMMDDHHMMSS_m<编号>_<说明>.sql`，时间要晚于已有的最新迁移。
- 模块集成测试放外部测试包（`package xxx_test`），需要内部函数时用 `export_test.go` 暴露。否则 testutil → app → 模块会循环引用。
- `db/models.go` 是生成文件，合并冲突时取任一边，再 `go generate ./...`。
- 事务直接用 `db.BeginTx`，连接已设 `_txlock=immediate`。
- 用 coder/websocket 写数据时，不要传会被取消的 context，它会关掉整条连接。
- 不要对 `web/src/api/gen/` 运行 prettier。
- 中文词典是全局的：同一个英文键在所有模块里必须对应同一个中文，意思不同就换一个英文键。`web/src/lib/i18n.test.ts` 会检查。

## Git 规则

- `main` 是线上版本，推送或合并到它会自动部署。只有用户同意才能动它。
- `develop`、`codex` 上最后一个提交带 `[deploy]` 的推送也会部署到线上，只在用户说“构建”“部署”时这样做（见上面的“构建和部署”）。
- `develop` 是唯一的开发分支，直接在上面提交。
- `codex` 只给 Codex 用，方便 Claude 审查。PR 合并后删掉，用的时候再从 `develop` 开。不要新开别的分支。
- `frontend-done` 是交给 Codex 之前的版本，不要改动或删除。
- 不要强推，不要改写已推送的历史。
