# 交接状态

正在做的一批任务有交接状态时写在这里。批次做完后把内容挪到 `docs/archive/`，这里只留下面的环境提示。

## 进行中：B220 邮件分拣结果推到今日页（2026-10-15 Claude 交接）

规格见 [B220](specs/B220.md)。用户要求：AI 分拣出的待办和需要我决定的邮件，自动出现在今日页对应的卡片里；今日页邮件卡片每封邮件的标题后面标上分拣类别。B136 原来的规则“AI 不因为邮件内容做任何事”已经由用户改掉，规格里写了。

### 做完的（在工作区，和这份文档一起提交，没有完整检查）

- 接口：`api/modules/mail.yaml` 加了 `MailTriage.decision`、`MailTriage.doneAt`、`GET /mail/todo`、`PUT /mail/messages/{id}/done`、`MailDone`。生成代码已经更新。
- 前端（Opus 5.5 写）：
  - `features/overview/components/MainCards.tsx`：“今日待办”加不用做决定的邮件，“待你决定”加要做决定的邮件。每行显示 AI 总结、发件人、邮件里的日期，勾一下调 `PUT /mail/messages/{id}/done`。
  - `features/overview/components/ExtraCards.tsx`：邮件卡片标题后面加类别标签，样式在 `overview.css`（`today-row-title`）。
  - `features/mail/triage.ts` 加 `triageLabel`，要做决定的邮件标“待决定”。邮件页列表和阅读区都改用它。
  - `features/mail/api.ts` 加 `useMailTodo`、`useSetMailDone`。
  - 测试：`TodayPage.test.tsx` 加了一条。前端类型检查和 overview、mail 的测试在交接前跑过，全部通过。
- 后端（Sonnet 5.5 子代理写，**没做完，也没审查**）：
  - 迁移 `20261015030000_m54_b220_mail_triage_decision.sql`：`mail_triage` 加 `decision`、`done_at`。
  - `triage.go`、`messages.go`、`queries.sql`、`triage_test.go`、新文件 `todo_test.go` 都改过。
  - 子代理停下前在查一个偶发失败的测试，它在旧代码副本上把邮件模块测试连跑了 7 遍，结论不知道。

### 接手要做的

1. 先跑 `cd backend && go vet ./internal/server/modules/mail/ && go test -race -count=1 ./internal/server/modules/mail/`，看后端能不能编译、测试过不过。
2. 对照规格“怎么做”和“验收”两节检查后端，重点看这几处：
   - 提示词和 JSON schema 有没有加 `decision`。
   - category 不是 action 时，decision 是不是强制为 false。
   - 重新分拣会不会清掉 `done_at`。
   - `/mail/todo` 是不是只返回 30 天内、最多 20 封。
   - `/mail/summary` 的 latest 有没有带 triage。
   - `triage.go` 开头关于“AI 不做任何事”的注释改了没有。
3. 缺的补上。别的模型写的代码要由 Opus 5.5 审查，见 AGENTS.md“用哪个模型写代码”。
4. 跑 `scripts/check-quick.sh`。截图看 `/` 和 `/mail` 两页：`npm run shots -- --only /,/mail`。
5. 在 `docs/tasks-done.md` 加 B220 一行，写清楚哪部分是哪个模型写的。从 `docs/tasks.md` 删掉 B220，并把这一节清空。

### 这次会话的其他结果（都已推到 develop）

- B217：已读、星标、删除改由常连的连接去做，用来修阿里企业邮箱已读又变回未读。没用真实阿里账号验证，见 `tasks.md` 的“已知问题”。
- B218：邮件页铺满右侧。
- B219：邮件批量标已读、未读、删除，以及全部已读。
- B222：所有页面铺满右侧，规则在 `styles/ui.css` 末尾和 `07-design.md` 第二节。
- 用户问过的 Jev（TypeSafe AI 的分类模型）还没回答完。结论：它只能做分类和打分，每百万输入 token 约 0.04 美元，输出不收费，分类部分比现在用大模型便宜很多。可它写不了总结，也提取不了日期。要用的话，得分两步：Jev 判断类别和要不要做决定，原来的模型只给“要处理”的邮件写总结、提日期。它 2026-09-15 才开放试用，SDK 小版本之间有不兼容的改动。用户还没决定要不要换。

## 接手时注意

- 装代码生成工具：见 AGENTS.md“常用命令”第一段。
- 后端需要 Go 1.26（`go.mod` 要求，本地旧版本会自动下载工具链）。
- 本地的 Chromium 和 playwright-core 版本不一致时，截图和端到端测试都加 `XC_SHOTS_BROWSER=/opt/pw-browsers/chromium`。
- 认证中间件只在 `/api/v1/mcp` 上看 `Authorization: Bearer`。代理连 `/agent/connect` 也用 Bearer，改中间件时别弄坏（B43 第一次写错过，全量测试里代理全连不上）。
- 截图要先起服务端：`XC_MASTER_KEY=$(go run ./cmd/server gen-key) XC_DEV=1 XC_DATA_DIR=<临时目录> XC_ADDR=127.0.0.1:8090 XC_WEB_DIR=<仓库>/web/dist go run ./cmd/server`，再 `npm run shots -- --base http://127.0.0.1:8090`。
- 这个仓库有时是浅克隆，推送前先 `git fetch --unshallow origin develop`，看清远端用到了哪个任务编号，免得撞号（这次 B158 到 B160、B221 都撞过）。
