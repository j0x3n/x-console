# 交接状态（B41 到 B48 这一批）

用户 2026-09-30 让 Claude 直接写 B41 到 B48 的代码，并要求实时更新这个文件。额度用完或会话中断时，接手的 AI 先读这个文件，从“当前”那一步继续。

## 规则

- 分支：`develop`（用户指定 Claude 在这个分支上开发）。
- 提交信息以任务编号开头。为了随时能交接，一个任务可能有多个提交，都以同一个编号开头。要撤掉某个任务：`git log --oneline --grep '^B41'` 找出全部提交，逐个 `git revert`。
- 每完成一个小步骤就提交并推送，同时更新下面的进度。
- 小步骤只跑 `scripts/check-quick.sh`。每个任务做完跑一次完整检查（见 AGENTS.md 常用命令）。
- 没有用户说“部署”，提交信息里不带部署标记。

## 顺序

B41 → B45 → B44 → B42 → B48 → B46 → B43 → B47（原因见 `docs/tasks.md` 的“任务说明”）。

## 进度

| 任务 | 状态 | 说明 |
| --- | --- | --- |
| B41 | 完成 | 全部检查、端到端、截图都过了 |
| B45 | 第一步完成，等用户真机测试 | 开关 `?safari=all`、`A` 到 `F`、`off`。用户测完把结果发回来后做第三步（定稿） |
| B44 | 完成 | 点击区域 44px 没做，记在“已知问题” |
| B42 | 完成 | 设置 → AI 用量；后端 `modules/ai/usage.go` |
| B48 | 完成 | 设置 → 安全；`auth.RequireStrictElevated` 给 B43、B47 用 |
| B46 | 后端完成，前端进行中 | 后端：迁移 `m5_b46_boards`，`projects/boards.go`、`boards_handlers.go`、`boards_actions.go`。前端要做：看板页签、自定义列表、卡片详情的成员/归档/复制/活动、星标、拖动 |
| B43 | 未开始 | |
| B47 | 未开始 | |

## 当前

B46：后端已提交（看板和列表的增删改、卡片按列表移动和跨项目移动、状态和列表联动、归档恢复、复制、成员、活动记录、看板动作 `projects.list_boards/get_board/create_card/move_card/comment/add_checklist_item/check_item/archive_card`）。
顺手修了一个旧问题：以前拖动卡片会清掉截止时间和分类（旧的 moveIssue 没传这些字段）。
下一步：前端 `features/projects`：项目页顶部看板页签、工具栏、自定义列表（改名、状态、WIP、归档）、卡片拖到任意列表、卡片详情加成员/归档/复制/活动；去掉 B36 的分类界面；侧边栏显示标星看板。
B45 等用户说“部署”后带部署标记推送，用户真机测试。

## 接手时注意

- 装代码生成工具：见 AGENTS.md“常用命令”第一段。
- 后端需要 Go 1.26（`go.mod` 要求，本地旧版本会自动下载工具链）。
- 本地的 Chromium 和 playwright-core 版本不一致时，截图和端到端测试都加 `XC_SHOTS_BROWSER=/opt/pw-browsers/chromium`。
- 截图要先起服务端：`XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1 XC_DATA_DIR=<临时目录> XC_ADDR=127.0.0.1:8090 XC_WEB_DIR=<仓库>/web/dist go run ./cmd/server`，再 `npm run shots -- --base http://127.0.0.1:8090`。
