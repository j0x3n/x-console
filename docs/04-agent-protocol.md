# 代理协议

代码在 `backend/pkg/protocol`（消息定义）和 `backend/pkg/rpc`（收发实现）。服务端的 `agenthub` 和代理端的 `conn` 都基于 `rpc.Peer`。

## 配对

1. 你在设置页生成配对码（要二次验证）。配对码形如 `K7QM-3XHP`，10 分钟有效，只能用一次。
2. 在目标机器上运行：
   ```
   x-console-agent pair --server https://console.example.com --code K7QM-3XHP
   ```
3. 代理调用 `POST /api/v1/agent/pair`，带上主机名、系统、架构、能力列表，拿到 `agentId` 和 `token`。
4. 代理把它们存到配置文件，权限 0600。默认位置是用户配置目录下的 `x-console-agent/config.json`。Linux 上以 root 运行时建议 `--config /etc/x-console-agent/config.json`。

## 连接

```
代理                                     服务端
 │ GET /api/v1/agent/connect               │
 │ Authorization: Bearer <token>           │
 │ ───────────── WebSocket 升级 ──────────> │
 │ {"k":"hello","p":{Hello}}   ──────────> │ 校验协议版本
 │ <────────── {"k":"welcome","p":{...}}   │
 │ <======== 请求 / 流 / 事件 双向 ========> │
```

- 协议版本不一致时，服务端用 1008 关闭连接，代理不再重连，提示升级。
- 令牌被吊销时，服务端回 401，代理退出并提示重新配对。
- 其他断线由代理自动重连，间隔 1 秒起步，翻倍，最长 1 分钟。
- 服务端每 30 秒发一次 WebSocket ping，10 秒没回就断开。
- 同一个代理重复连接时，旧连接被关闭。

## 帧格式

每一帧是一个 JSON 对象（`protocol.Envelope`）：

| 字段 | 含义 |
| --- | --- |
| `k` | 帧类型：hello、welcome、req、res、cancel、open、data、end、event |
| `id` | 请求或流的 id。服务端发起的以 `s` 开头，代理发起的以 `a` 开头 |
| `m` | 方法名或事件名 |
| `p` | 参数 |
| `r` | 结果 |
| `e` | 错误 `{code, message}` |
| `d` | 流数据，字节，JSON 里是 base64 |

## 三种交互

### 请求和响应

```go
// 服务端
var out protocol.ProcessList
err := hub.Call(ctx, agentID, protocol.MethodProcList, protocol.ProcListParams{Limit: 100}, &out)

// 代理端
client.Handle(protocol.MethodProcList, func(ctx context.Context, raw json.RawMessage) (any, error) {
    var p protocol.ProcListParams
    if err := json.Unmarshal(raw, &p); err != nil {
        return nil, &protocol.Error{Code: protocol.CodeBadParams, Message: err.Error()}
    }
    return listProcesses(ctx, p)
})
```

- `hub.Call` 默认 30 秒超时。ctx 取消时会给代理发 `cancel`，代理端处理函数的 ctx 随之取消。
- 代理返回的错误会变成 `httpx.Error`：`unsupported` 和 `unknown_method` 对应 501，`bad_params` 对应 400，`timeout` 对应 504，其他对应 502。代理不在线对应 503 `agent_offline`。
- 当前系统不支持的功能返回 `&protocol.Error{Code: protocol.CodeUnsupported}`。

### 流

用于终端、跟随日志、编码任务输出、文件传输。

```go
// 服务端
s, err := hub.Open(ctx, agentID, protocol.MethodPTYOpen, protocol.PTYOpenParams{Cols: 120, Rows: 32})
defer s.Close(nil)
go func() { for { b, err := s.Recv(ctx); if err != nil { return }; browser.Write(b) } }()
s.Send(ctx, keystrokes)

// 代理端
client.HandleStream(protocol.MethodPTYOpen, func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
    // 读 s.Recv，写 s.Send 或 s.Write。返回时流自动关闭，返回的错误会传给对方。
})
```

- `Recv` 在对方正常结束时返回 `io.EOF`，异常结束时返回 `*protocol.Error`。
- 任何一方调用 `Close` 都会结束整条流。`s.Context()` 在流结束时取消。
- 控制信息（比如终端窗口大小变化）和数据共用一条流时，自己约定格式。终端的约定见 M2 文档。

### 事件

代理主动上报，没有 id，不需要回复。

```go
// 代理端，默认每 30 秒推概要；收到 metrics.detail {on:true} 后按 intervalMs（默认 5 秒）推详情
client.OnConnect(func(ctx context.Context) {
    t := time.NewTicker(30 * time.Second)
    for { select { case <-ctx.Done(): return; case <-t.C: client.Emit(ctx, protocol.EventMetrics, sample()) } }
})

// 服务端，在模块 New 里注册
d.Agents.OnEvent(protocol.EventMetrics, func(agentID string, raw json.RawMessage) { ... })
```

事件处理函数在连接的读循环里执行，必须很快返回。耗时的工作丢到 channel 里异步处理。

样本和概要里的 `netRxTotal`、`netTxTotal` 是开机以来的累计收发字节，只算真实网卡（排除 `lo`、docker、veth、网桥、虚拟机和隧道网卡，规则在 `protocol.CountedInterface`）。服务端用相邻两次的差算月流量：计数变小说明重启过，这一步算 0；两次之间超过 10 Gbit/s 的差值丢弃；服务端重启后的第一个样本只当起点。不带这两个字段的旧代理，服务端按网速估算，最多补 60 秒，并标成“估算”。

`metrics.detail` 是服务端发给代理的请求。参数为 `{ "on": true, "intervalMs": 1000 }` 或 `{ "on": false }`。`intervalMs` 只接受 1000、5000、30000，不传、为 0 或别的值都按 5000。不认识这个字段的旧代理一直按 5 秒上报，服务端不需要特殊处理。1 秒模式下代理每秒只重新读 CPU、内存、网速和磁盘读写，每核 CPU、磁盘容量、网卡列表、交换区和进程数每 5 秒读一次，中间的样本沿用上一次的值。详情指标包含每块磁盘和每张非回环网卡的速率。服务端在最后一个详情订阅者离开 60 秒后关闭详情模式。代理重连后先上报概要，服务端会按当前订阅状态重新开启详情。

## 能力

代理在 hello 里上报能力列表，常量在 `protocol.Cap*`。前端据此隐藏做不到的操作。

| 能力 | Linux | Windows |
| --- | --- | --- |
| system.info、metrics、processes、files、exec、pty | 有 | 有 |
| services | systemd | Windows 服务 |
| docker | 装了 Docker 才有 | 一般没有 |
| clipboard、power、coding | 没有 | 有 |
| proxy（访问代理所在内网的 HTTP 和 WebSocket，只允许私有地址） | 有 | 有 |

## 新增方法的步骤

1. 在 `pkg/protocol/methods_<模块>.go` 里定义方法名常量、参数和结果结构体。
2. 在 `internal/agent/<功能包>/` 里实现，平台相关代码用 `_linux.go`、`_windows.go` 或 build tag 分开，另写一个 `_other.go` 返回 `CodeUnsupported`。
3. 在 `cmd/agent/main.go` 的 `register()` 里加一行，在 `capabilities()` 里按系统加上能力。
4. 服务端模块通过 `d.Agents.Call` 或 `Open` 调用。
5. 测试：代理端功能包写单元测试；服务端用 `testutil.Env.Agent` 挂一个假实现做集成测试。
