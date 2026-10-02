# B97 个人计划检查

原文件：`个人提示计划.standalone.html`。静态内容已经提取到习惯模块，页面使用项目的 React 组件。原文件没有写入项目。

- 66 个动作，包含 38 个基础动作和 28 个补充动作。保留训练量、图解、步骤、常见错误、简化和进阶说明。
- 9 个力量模板、3 个手动阶段、9 个走跑级别。
- 16 个习惯模板。12 个来自原计划，4 个来自体态补充。
- 26 节日常、饮食、英语和参考资料。完整原始对话和训练附件可阅读。
- 68 张 WebP 图解，合计约 5.3 MB。静态文字随后端嵌入，图解随前端发布。

## 检查结果

- 后端生成和格式检查通过。生成后没有新增差异。
- Linux 下 `go vet ./...` 和全部 `go test -race ./...` 通过。测试进程使用 UTC，与 CI 一致。
- Windows 代理构建和 vet 通过。
- 前端类型生成、类型检查、构建、格式和字号检查通过。
- Node 22 下全部 76 个测试文件、454 项测试通过。
- 真实服务端和 Linux 代理的完整端到端流程通过。新增流程覆盖模板加入、打卡、撤销、刷新、身体记录、跟练保存、重复保存、动作选择、周计划和备份合并。
- 全页截图在 1360px 和 390px 下检查通过。最终个人计划和健身页面再次检查了深浅主题。没有页面异常或横向溢出。
- 后端测试覆盖导入事务、失败回滚、并发重复打卡、外部删除和 SQLite ID 重用。训练量的区间、秒和米原样保存。
- 个人计划默认日期使用习惯作息的时区。前端测试覆盖 UTC 与上海日期跨日。

本机 Node 25 的存储兼容问题和 Windows 字号脚本路径问题已有记录。本次使用 Node 22 和 Linux 完成检查。没有修改这些公共脚本。

## 页面截图

以下每页都有桌面、手机、深色和浅色截图。

| 页面 | 浅色桌面 | 浅色手机 | 深色桌面 | 深色手机 |
| --- | --- | --- | --- | --- |
| 动作库 | [查看](light/1360/habits-fitness.png) | [查看](light/390/habits-fitness.png) | [查看](dark/1360/habits-fitness.png) | [查看](dark/390/habits-fitness.png) |
| 计划概览 | [查看](light/1360/habits-personal-overview.png) | [查看](light/390/habits-personal-overview.png) | [查看](dark/1360/habits-personal-overview.png) | [查看](dark/390/habits-personal-overview.png) |
| 跟练 | [查看](light/1360/habits-personal-training.png) | [查看](light/390/habits-personal-training.png) | [查看](dark/1360/habits-personal-training.png) | [查看](dark/390/habits-personal-training.png) |
| 日常习惯 | [查看](light/1360/habits-personal-daily.png) | [查看](light/390/habits-personal-daily.png) | [查看](dark/1360/habits-personal-daily.png) | [查看](dark/390/habits-personal-daily.png) |
| 饮食 | [查看](light/1360/habits-personal-food.png) | [查看](light/390/habits-personal-food.png) | [查看](dark/1360/habits-personal-food.png) | [查看](dark/390/habits-personal-food.png) |
| 英语 | [查看](light/1360/habits-personal-english.png) | [查看](light/390/habits-personal-english.png) | [查看](dark/1360/habits-personal-english.png) | [查看](dark/390/habits-personal-english.png) |
| 身体记录 | [查看](light/1360/habits-personal-records.png) | [查看](light/390/habits-personal-records.png) | [查看](dark/1360/habits-personal-records.png) | [查看](dark/390/habits-personal-records.png) |
| 设置与备份 | [查看](light/1360/habits-personal-settings.png) | [查看](light/390/habits-personal-settings.png) | [查看](dark/1360/habits-personal-settings.png) | [查看](dark/390/habits-personal-settings.png) |
| 原文与参考 | [查看](light/1360/habits-personal-reference.png) | [查看](light/390/habits-personal-reference.png) | [查看](dark/1360/habits-personal-reference.png) | [查看](dark/390/habits-personal-reference.png) |

## 迁移旧记录

原 HTML 的打卡、组数和身体记录保存在原浏览器的 `daily-plan-v1`。这些记录不在 HTML 文件中。用户需要在原页面设置里导出 JSON，再到项目的“个人计划 → 计划设置”导入。

个人 JSON 保留设置、勾选、身体记录和笔记。已经保存到原生模块的训练记录由项目的数据库备份保存。

本批没有增加数据库迁移，也没有部署。
