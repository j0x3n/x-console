# 文档索引

协作规则、常用命令和流程在根目录的 [AGENTS.md](../AGENTS.md)。进度和下一步在 [tasks.md](tasks.md)。

| 文档 | 内容 | 什么时候看 |
| --- | --- | --- |
| [tasks.md](tasks.md) | 任务看板、已知问题、接口变更记录 | 每次开始工作前 |
| [backend-todo.md](backend-todo.md) | B22 到 B37 的后端按什么顺序做，还剩哪些 501 接口 | 开始做这批后端前 |
| [specs/](specs/) | 每个模块和需求的规格：数据表、接口、页面、验收标准 | 做某个任务前 |
| [01-product.md](01-product.md) | 产品是什么，每个模块做到什么程度算完成 | 想了解全貌时 |
| [02-architecture.md](02-architecture.md) | 各部分怎么配合，已定的技术决定，安全 | 想了解全貌时 |
| [03-backend.md](03-backend.md) | 后端约定：新增模块的步骤、处理器写法、基础服务 | 写 Go 之前 |
| [04-agent-protocol.md](04-agent-protocol.md) | 代理配对、连接、请求、流、事件 | 写代理或调用代理之前 |
| [05-frontend.md](05-frontend.md) | 前端约定：模块结构、调接口、组件和样式 | 写前端之前 |
| [06-deploy.md](06-deploy.md) | 部署、代理安装、本地联调 | 部署或排查线上问题时 |
| [07-design.md](07-design.md) | 界面设计规范：页面结构、组件清单、数值、自查和截图 | 做任何界面之前 |

## 仓库结构

```
AGENTS.md       协作规则（AI 工具会自动加载）
api/            接口契约。common.yaml 放共用结构，modules/<模块>.yaml 每个模块一份
backend/        Go 代码，一个 Go 模块
  cmd/server    服务端入口
  cmd/agent     代理入口，Linux 和 Windows 用同一份代码
  internal/server  服务端：基础服务 + modules/<模块>
  internal/agent   代理端：连接 + 各功能包
  pkg/protocol  服务端和代理之间的消息定义
  pkg/rpc       在一条连接上跑请求、流和事件
web/            前端，React 19 + Vite + TypeScript
deploy/         Docker、Caddy、部署脚本、代理安装脚本
docs/           本目录
.github/        CI、自动部署、PR 模板
```
