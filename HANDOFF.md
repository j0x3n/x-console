# X Console 交接

更新时间：2026-09-27

## 现状

- 产品方向、技术选型、模块划分已和用户确认，见 `docs/01-product.md`。
- 批次 0 已完成：Go 后端基础（登录、TOTP、会话、审计、加密设置、事件、调度、通知、代理配对与协议）、前端外壳（登录、导航、命令面板、通知、设置页）、全部开发文档。
- 旧的 CRM 原型页面已删除。旧代码可以用 `git show 3215f0f:<路径>` 查看，比如看板组件 `src/features/work/components/WorkBoard.tsx`。

## 下一步

按 `docs/roadmap.md` 的批次推进。每个任务卡由一个子代理在独立 worktree 里开发，完成后合并到 `claude/focused-wozniak-i2fda9`。

## 恢复开发时

1. 读 `docs/README.md` 和 `docs/roadmap.md`，看哪个批次在进行。
2. `cd backend && go test ./...`，`cd web && npm ci && npm test && npm run build`，确认基线是好的。
3. 继续下一个任务卡。

## 用户的环境

- 本机是 Windows（项目原来在 `C:\Users\xcc19\Documents\ChatGPT\个人中心`）。
- 服务器是 1 到 10 台 Linux。
- 通知渠道：站内、Web Push、Telegram、Bark、Server酱。
