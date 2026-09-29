# 后端待做总清单（B22 到 B37）

## 进度区（谁做谁更新，每做完一小步就改这里）

接手的人先看这一节。分支 `claude/project-thread-37jvjz`，草稿 PR 到 `develop`。用户 2026-09-29 定：先做前一半（第一组到第三组），做到哪算哪。

状态只有三种：没开始 / 在做 / 做完。

| 序号 | 任务 | 状态 | 在做的文件 | 下一步 |
| --- | --- | --- | --- | --- |
| 1 | B22 偏好设置 | 做完 | `core/preferences.go` | 无 |
| 2 | B23 天气、订阅分类、周期 | 做完 | `brief/weather.go`、`monitoring/categories.go`、`monitoring/subscriptions.go`、迁移 `20260929000100` | 无 |
| 3 | B34 推送自检 | 没开始 | | |
| 4 | B35 GitHub | 没开始 | | |
| 5 | B24、B25 文件目录和备份 | 没开始 | | |
| 6 | B26 服务器详情 | 没开始 | | |
| 7 | B27 月流量 | 没开始 | | |
| 8 | B28 容器日志、镜像清理 | 没开始 | | |
| 9 | B29 系统日志 | 没开始 | | |
| 10 | B30 一条命令添加服务器 | 没开始 | | |
| 11 | B33 远端日志部分 | 没开始 | | |
| 12 起 | B31 及以后 | 不在前一半 | | |

本地跑检查要装工具：`go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1`、`go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0`，然后照 `AGENTS.md` 的命令跑。


2026-09-29 整理。这一批的前端都已经做完，后端还没做的接口现在回 501，前端显示“还没上线”或者退回旧的做法。开发者按下面的顺序做，每个任务的细节在对应规格的“后端（待做，给开发者）”一节。

规则照 `AGENTS.md`：在 `codex` 分支上做，一个任务一个提交，提交信息以编号开头。做完一个接口，把它从对应的 `pending.go` 删掉；一个模块的都做完了，删掉整个 `pending.go`。

## 先看这里

- 接口契约已经写好，在 `api/modules/*.yaml`，生成的代码已经提交。不要改契约里的字段名，前端按它写好了。确实要改时，先在 PR 里说明，前端一起改。
- 501 的接口都在这几个文件里，数一数就知道还剩多少：

| 文件 | 接口数 | 任务 |
| --- | --- | --- |
| `modules/monitoring/pending.go` | 6 | B23、B28 |
| `modules/hosts/pending.go` | 7 | B27、B29、B33 |
| `modules/drive/pending.go` | 21 | B31 |
| `modules/ai/pending.go` | 14 | B32、B33 |
| `modules/notes/pending.go` | 3 | B32 |
| `modules/reminders/pending.go` | 4 | B34、B37 |
| `modules/github/pending.go` | 1 | B35 |
| `modules/projects/pending.go` | 12 | B36 |

`drive/pending.go` 里还有一个 `PublicPaths`，登记分享页不用登录的路径。做完 B31 以后它要留着，挪到模块的正式文件里。

- 还有两个契约没有后端模块，现在回 404：`api/modules/storage.yaml`（B24、B25）和 `api/modules/files.yaml`（B36 的公共上传）。
- 迁移只加不删。文件名里的时间按实际写，要晚于当时最新的迁移。规格里写的文件名只是示意。

## 顺序

按依赖排。同一组里的可以换顺序。

### 第一组：零碎的，先做

1. **B22 主题色和偏好设置**：`GET/PUT /me/preferences`。规格 `specs/B22.md`。
2. **B23 天气刷新、订阅分类、周期**：规格 `specs/B23.md`。
3. **B34 推送自检**：订阅加三列，记每次推送的结果，订阅列表、按 id 删除、测试接口。规格 `specs/B34.md`。
4. **B35 GitHub**：仓库列表（分页、缓存 10 分钟）、同步改成 1 分钟（额度不够退回 5 分钟）、PR 分页。规格 `specs/B35.md`。

### 第二组：存储

5. **B24、B25 统一文件目录和备份**：`files.Store`、S3、导出导入、自动备份。规格 `specs/B24-B25.md`。后面云盘版本、公共上传都会用到它；没做完时它们先写本地目录，规格里写了怎么过渡。

### 第三组：服务器和代理

6. **B26 服务器详情和刷新周期**：规格 `specs/B26.md`。
7. **B27 月流量**：代理上报累计字节数，服务端算增量。规格 `specs/B27.md`。
8. **B28 容器日志来源、镜像清理**：规格 `specs/B28.md`。
9. **B29 系统日志**：代理加 journalctl 和 Windows 事件日志。规格 `specs/B29.md`。
10. **B30 一条命令添加服务器、Windows 客户端**：规格 `specs/B30.md`。
11. **B33 的远端日志部分**：代理 `files.read` 加 `Offset`、`Length` 和能力 `files.range`，服务端 `files/range`、`files/follow`。规格 `specs/B33.md` 第 7 节。可以和 B31 的日志实时共用分帧代码，所以放在 B31 前后都行。

### 第四组：云盘

12. **B31 云盘**：打包下载、后台任务、复制移动、压缩解压、历史版本、外链分享（公开入口要校验）、日志实时。规格 `specs/B31.md`。

### 第五组：AI

13. **B32 AI 改成 OpenAI 兼容接口**：供应商、models.dev 规格、调用层 `modules/ai/llm`、改 AI 浮窗和自动化、模型设置、用量、笔记自动标题和标签。规格 `specs/B32.md`。这一步删掉 `anthropic-sdk-go`。
14. **B33 服务器的 Agent 标签**：依赖 B32 的调用层。工具、命令风险判断、确认规则、审计。规格 `specs/B33.md` 第 1 到 6 节。

### 第六组：项目和提醒

15. **B36 项目**：分类、Issue 新字段、检查清单、到期提醒、公共上传模块 `modules/files`。规格 `specs/B36.md`。
16. **B37 提醒页汇总**：`contracts.ReminderSource`，监控和项目各实现一个。项目那边依赖 B36 的 `due_at`。规格 `specs/B37.md`。

清理轮 C3、C4、C5 照 `docs/tasks.md` 的位置穿插。

## 安全要点（验收时重点看）

- 要提升权限（`auth.RequireElevated`）的：改 AI 供应商和模型设置、机器 Agent 改成“全部自动”、机器 Agent 执行高危命令、云盘外链分享的创建、改 GitHub 令牌。
- 加密存储（`secrets.Box`）的：AI 供应商的 Key、S3 的密钥、GitHub 令牌（原来就是）。接口只返回 `hasApiKey` 这类字段，不返回明文。
- 公开入口：云盘分享页 `/public/shares/*` 要校验提取码和次数；Telegram 回调原来就校验密钥。
- 机器 Agent：只读判断用白名单，判断不出来一律当写操作；高危命令在任何权限下都要确认；每条执行都记审计，actor 是 `ai:host-agent`。
- 上传：按文件内容判断类型（`http.DetectContentType`），不信文件名和客户端给的类型；限制大小。

## 做完一个任务后

- 删掉对应的 501 函数，跑 `AGENTS.md` 里的全部检查。
- 起真实服务端，打开对应页面看“还没上线”的提示消失了，功能能用。
- 在 `web/scripts/e2e.mjs` 补规格里写的主流程。
- `docs/tasks.md` 把任务移到“已完成”，写上提交号；“已知问题”里相关的条目删掉。
