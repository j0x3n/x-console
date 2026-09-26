# X Console 开发文档

X Console 是一个人用的控制台。它管服务器和 Windows 本机，跑本机仓库的编码任务，管项目、备忘、提醒和习惯，还接 Home Assistant、GitHub 和 Claude。

## 阅读顺序

| 顺序 | 文档 | 什么时候看 |
| --- | --- | --- |
| 1 | [01-product.md](01-product.md) | 想知道做什么、做到什么程度算完成 |
| 2 | [02-architecture.md](02-architecture.md) | 想知道各部分怎么配合、安全怎么做 |
| 3 | [03-backend.md](03-backend.md) | 写 Go 代码之前必看 |
| 4 | [04-agent-protocol.md](04-agent-protocol.md) | 写代理程序或调用代理之前必看 |
| 5 | [05-frontend.md](05-frontend.md) | 写前端之前必看 |
| 6 | [06-deploy.md](06-deploy.md) | 部署、本地联调 |
| 7 | [modules/](modules/) | 每个模块的数据表、接口、页面和验收标准 |
| 8 | [roadmap.md](roadmap.md) | 开发批次和子代理任务卡 |

## 仓库结构

```
api/            接口契约。common.yaml 放共用结构，modules/<模块>.yaml 每个模块一份
backend/        Go 代码，一个 Go 模块
  cmd/server    服务端入口
  cmd/agent     代理入口，Linux 和 Windows 用同一份代码
  internal/server  服务端：基础服务 + modules/<模块>
  internal/agent   代理端：连接 + 各功能包
  pkg/protocol  服务端和代理之间的消息定义
  pkg/rpc       在一条连接上跑请求、流和事件
web/            前端，React 19 + Vite + TypeScript
deploy/         部署文件
docs/           本目录
```

## 常用命令

```bash
# 后端
cd backend
go generate ./...        # 重新生成 sqlc 和 oapi-codegen 代码
go vet ./... && go test ./...
GOOS=windows GOARCH=amd64 go build ./cmd/agent   # 必须能交叉编译

# 前端
cd web
npm ci
npm run gen:api          # 从 api/modules/*.yaml 生成 src/api/gen/*.ts
npm run typecheck && npm test && npm run build
npm run dev              # 开发服务器，/api 转发到 127.0.0.1:8080
```

代码生成工具需要单独安装一次：

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0
```
