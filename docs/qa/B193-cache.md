# B193 AI 缓存检查

## 修改前

本地没有线上用量数据库。下面的比例来自现有测试数据，只用于核对算法，不能当成线上效果。

按用途统计时，用 `sum(cached_input_tokens) / sum(input_tokens)`。不要平均每次调用的命中率。输入为零时不返回比例。缓存写入量不算命中量。

`ai/usage_test.go` 的样例：

| 用途 | 输入 token | 缓存命中 token | 命中率 |
| --- | ---: | ---: | ---: |
| notes | 100 | 80 | 80% |
| assistant | 100 | 20 | 20% |
| coding（外部用量样例） | 1000 | 900 | 90% |

assistant 样例最低。样例是人工插入的数据，不能据此判断线上原因。上线后可按相同时间范围查询：

```sql
SELECT source, purpose, count(*) AS calls,
       sum(input_tokens) AS input_tokens,
       sum(cached_input_tokens) AS cached_input_tokens,
       1.0 * sum(cached_input_tokens) / nullif(sum(input_tokens), 0) AS cache_hit_rate
FROM ai_usage
WHERE created_at >= ? AND created_at < ?
GROUP BY source, purpose
ORDER BY cache_hit_rate;
```

代码里的前缀变化点：

| 用途 | 现状 | 计划 |
| --- | --- | --- |
| assistant | `system()` 已固定。`panelSystem()` 仍拼接记忆内容。`tools(visible)` 会随隐藏模块锁定状态改变 | 记忆保留在 system 末尾。工具按当前上下文过滤，同一种锁定状态下保持顺序稳定 |
| host_agent | 每轮查机器名称、能力和实时系统信息。离线或调用失败会改变 system，能力列表没有排序 | 固定规则和机器 id 放在 system，记忆接在末尾。机器信息放本轮用户上下文，能力排序 |
| 内置 Agent | 工具定义也使用带隐藏状态的 context。单次执行的 system 由调用方提供 | 保留当前上下文的可见性过滤，工具按名称排序 |
| polish | 用户自定义要求拼进 system | 放进用户消息，保留场景固定说明 |
| notes | 润色要求和已有标签拼进 system。自动标题标签也有同样问题 | 固定说明留在 system，要求和标签放进用户消息 |
| brief、cards、inbox、dayplan、mail、knowledge、readlater、automation | 固定说明已在 system。早报、选中文字、候选数据、邮件和触发数据已在用户消息 | 保持现有拼接 |
| 所有模型请求 | chat 和 responses 都没有 `prompt_cache_key` | 会话用会话标识，其余用用途。不支持时移除并重试 |

## 修改结果

已完成：

- 面板和内置 Agent 的工具定义按当前上下文过滤。锁定时不发送隐藏模块的工具名和说明。目录仍按名称排序，同一种锁定状态下保持一致。执行时也检查可见性。
- 面板和服务器 Agent 的记忆放在 system 末尾。用户消息只保存本轮动态信息，不重复保存完整记忆，也不附加关闭记忆的提示。
- 服务器 system 保留固定规则、机器 id 和末尾的记忆。名称、系统、能力、在线状态、系统信息和指标放进本轮用户消息。能力列表先复制再排序。
- 早先保存的消息不重写。新机器信息只追加到本轮消息。新记忆在下一轮生成时更新 system 末尾。
- chat 和 responses 的流式、普通请求都发送 `prompt_cache_key`。面板和机器会话用来源加会话 id。内置任务用执行来源和 ref。其他用途用来源，未指定来源时用 purpose。
- 供应商返回 400 并提到不支持缓存键时，移除字段重试。最多四次请求，能依次处理缓存键、推理字段和 JSON Schema 的兼容性问题。
- AI 润色和笔记润色的额外要求移到用户消息。手动标签建议和自动标题标签的已有标签也移到用户消息。

这两项按审查意见恢复。隐藏模块的工具定义即使不能执行，也会让模型知道模块名称和用途。模型回答“能做什么”时可能泄露这些信息。锁定和解锁切换很少发生，接受此时缓存失效。

记忆也很少变化。放在 system 末尾只在记忆变化时使后面的缓存前缀失效。每条用户消息都存一份完整记忆，会让长对话反复发送这些内容。恢复到 system 可以减少输入 token。

验证结果：

- `ai/cache_test.go`：锁定、解锁和再次锁定时，各验证同一状态下连续两轮工具列表相同。锁定时没有隐藏模块的工具。机器信息变化不改变 system，能力排序稳定。缓存键不变，旧历史逐项相等。
- 同一文件验证面板和内置 Agent 都不发送锁定模块的工具定义，也不能执行这些动作。
- 记忆放在面板和服务器 system 末尾。同一份记忆下连续两轮 system 相同。记忆修改只更新末尾内容，历史不重复保存记忆。关闭记忆后不发送记忆或关闭提示。
- `llm/cache_test.go`：chat、responses 各验证流式和普通请求。覆盖两轮请求体、会话缓存键、用途默认键、移除缓存键重试、三种不支持字段连续回退。
- `notes/ai_test.go`：润色要求和标签变化不改变 system。用户消息仍包含这些内容。自动标题标签也验证两轮请求。
- `polish_test.go` 保留额外要求在用户消息的断言。`memory_test.go` 恢复 system 中记忆的断言。
- 返工检查：`go vet ./internal/server/modules/ai/...` 和 `go test ./internal/server/modules/ai/... ./internal/server/modules/notes/...` 全部通过。改动文件的 `gofmt -l` 无输出。

尚未验证真实供应商的缓存命中率。前缀稳定只是提高命中的必要条件。实际比例还受供应商的长度门槛、缓存保留时间和路由规则影响。部署后应按上面的 SQL 对比同一时间范围。
