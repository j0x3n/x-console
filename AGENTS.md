# X Console：给开发者（AI 编码工具）的说明

开始任何工作前，按顺序读：

1. `HANDOFF.md`：项目的完整上下文、当前进度、验证命令、踩过的坑、交付约定。
2. `docs/README.md`：开发文档索引。写 Go 之前读 `docs/03-backend.md`，写前端之前读 `docs/05-frontend.md`，涉及代理读 `docs/04-agent-protocol.md`。
3. `docs/backlog.md`：用户新提的需求，优先做。然后是 `docs/roadmap.md` 里的批次 3。

## 必须遵守

- 回复用户和界面文案都用中文：短句，一句一个意思，不用破折号插入语，不用比喻。
- 只改自己模块的目录；共享文件只加行；基础代码尽量不动，改了要在交付说明里写理由（规则见 `docs/roadmap.md`）。
- 提交前跑完 `HANDOFF.md` 第 7 节的全部验证命令。
- 每完成一批，写交付说明到 `docs/deliveries/<日期>-<名称>.md`（格式见 `HANDOFF.md` 第 13 节），交给 Claude 验收。
- 在分支 `claude/focused-wozniak-i2fda9` 上提交并推送。不要合并进 `main`，不要推提交信息带 `[deploy]` 的提交，除非用户同意。
