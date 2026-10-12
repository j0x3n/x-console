# B192 Apple 日历同步

## 修复

`go-webdav` 的 `QueryCalendar` 对每个 response 调用 `DecodeProp(calendar-data)`。任一条目的属性返回 404 时，会直接返回错误。后面的正常日程也不会读取。

现在自己发送 calendar-query REPORT。只读取成功 propstat 中的 calendar-data 和 ETag。跳过缺少日历数据、响应失败或内容无法解析的条目。只要有可读条目就继续同步。所有非空返回都不可读时记录错误，并保留旧日程。真正的空日历可以清除旧日程。

响应保留 href 和完整 ETag。写入路径使用 PUT，必要时 GET 回读。删除使用 DELETE。这些路径不会遇到 calendar-data propstat 的解析问题。另修复了写入后 ETag 引号被库去掉的问题。后续 If-Match 使用原始响应头。

旧版本保存的不带引号的 ETag 会在发送 If-Match 时补上引号。不需要数据库迁移。

## 样例与测试

样例在 `calendar/testdata/icloud-calendar-query-404.xml`。按 iCloud 的 XML 格式构造。没有真实账号原始响应。

- 用原来的 QueryCalendar 读取样例，复现完全相同的 calendar-data 404 错误。
- 用修复后的同步接口读取样例，两个正常条目成功入库。
- 验证分开的成功 propstat、其他属性的 404、资源级 404 和转义的 href。
- 验证全部条目 404 和 XML 不完整时保留旧日程。空 multistatus 可以清空日程。
- 验证同步后的条目可以连续修改和删除。PUT 未返回 ETag 时会 GET 回读，后续 If-Match 保留引号。

没有连接真实 iCloud 账号。线上仍需确认实际账号的完整同步和写回。

## 检查结果

`gofmt -l` 无输出。`go vet ./internal/server/modules/calendar/...` 和 `go test ./internal/server/modules/calendar/...` 均通过。没有跑全量测试、前端构建或端到端。

同工作目录的 `.git/index.lock` 无法创建。按用户要求保留工作目录中的改动。没有提交号，也没有推送。待提交信息的末行需加 `Co-Authored-By: Codex <noreply@openai.com>`。
