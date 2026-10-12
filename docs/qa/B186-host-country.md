# B186 服务器国旗排查

## 代码依据

- curl 安装后的代理通过 `/api/v1/agent/connect` 连接。`agenthub/hub.go` 在 WebSocket 握手时调用 `RequestIP`。收到 hello 后会保存 `sourceIPs[agent.ID]`。
- `agenthub/source.go` 复用 `httpx.ClientIP`。直连来源不在 `XC_TRUSTED_PROXIES` 时会忽略 `X-Forwarded-For`。Caddy 的内网地址会被当作来源地址。
- `config/config.go` 从环境变量读取可信代理。默认值为空。`deploy/docker-compose.yml` 使用 `.env`，没有自动设置这个变量。是否为本次线上故障仍需检查实际环境。
- `cmd/agent/main.go` 已调用 `sysinfo.RegisterAddresses`。代理每次连接后会请求 `/api/v1/agent/whoami`。服务端入口也在这个路径。不是前缀不一致。
- `hosts/hosts.go` 的详情请求和 `host_addresses.go` 的后台刷新都会合并网卡地址与 `Agents.SourceIP`。内网 IP 不参与国家查询。SSH 主机使用配置的地址，所以不依赖代理连接的来源。
- 旧实现只查询排序后的第一个公网 IP。这个地址没有国家记录时，后面的公网 IPv4 和 IPv6 都不会被查询。现已改为逐个尝试，并保存实际查到国家的 IP。
- 国家库暂时查不到时，仍在当前列表中的旧国家结果会保留。旧 IP 不再出现时会清除旧国家，避免显示过期结果。
- 国家库已经支持 IPv6。MaxMind 测试库中的 `2001:218::` 可查询到日本。
- 下载失败后已有一小时重试。后台每五分钟刷新地址，详情请求也会触发查询。本次补了失败后重试成功的测试，没有修改重试间隔。

## 线上需要检查

先打开缺国旗的主机详情，再查数据库。请提供这些结果。

```sql
SELECT a.id, a.name, a.hostname, a.kind, a.version,
       h.addresses, h.country_code, h.country_ip, h.country_checked_at
FROM agents AS a
LEFT JOIN host_info AS h ON h.host_id = a.id
WHERE a.kind = 'server' AND a.revoked_at IS NULL;
```

还需检查：

1. x-console 进程实际收到的 `XC_TRUSTED_PROXIES` 值。只看这个变量即可。
2. Caddy 转发给后端时的实际来源 IP，以及发送的 `X-Forwarded-For`。同机反代可能是 `127.0.0.1`，独立容器可能是 Docker 网段内的地址。
3. `/data/geo/country.mmdb` 是否存在、文件大小和修改时间。默认数据目录为 `/data`。若自定义了 `XC_DATA_DIR`，检查对应目录。
4. 缺国旗主机上调用 `/api/v1/agent/whoami` 的响应。使用该主机的有效代理令牌鉴权，只需返回的 `ip`。`SourceIP` 只保存在内存中，`agents` 表没有对应列。

若只有内网地址且 whoami 返回 Caddy 的 IP，就需要在部署环境中设置 `XC_TRUSTED_PROXIES`。填后端实际看到的 Caddy IP 或受控的代理网段。不要只按示例填一个 Docker 地址。设置后重新创建 x-console 容器，并让代理重连。详情请求会重新保存国家。

若已有公网地址但 `country_code` 为空，需检查国家库。库下载成功后再次打开详情。第一个公网地址没有记录时，修复后的代码会继续尝试其余地址。所有公网地址都无记录时仍不会显示国旗。

## 本地验证

- 测试复现首个公网地址无记录导致国旗缺失。覆盖 IPv4 回退、IPv6 回退和只有公网 IPv6。
- 测试覆盖内网网卡配合可信代理来源。未配置可信代理时不会采信转发头，配置后详情和列表可显示国家。
- 测试覆盖查询失败时保留当前地址的国家，以及旧地址消失时清除旧国家。
- 测试覆盖国家库下载失败后的重试，以及下载失败时保留已有库。

没有连接线上主机。没有更改部署配置。

## 检查结果

`gofmt -l` 无输出。`go vet ./internal/server/modules/hosts/...` 通过。新增的国家、地址与下载重试用例均通过。

`go test ./internal/server/modules/hosts/...` 未全通过。失败用例为 `TestExec`、`TestActions` 和 `TestAlertRulesFireAndResolve`。前两个用例收到 Windows 的 CRLF，断言要求 LF。告警用例的查询返回空列表。

用 Go overlay 临时替换为 Git HEAD 中原有的 `host_addresses.go` 后，三个用例仍以相同原因失败。没有覆盖工作目录中的文件。没有修改这些用例或告警代码。

只对本任务文件执行了 `git add`。创建 `.git/index.lock` 时被拒绝。没有提交号，也没有推送。

## 部署改动（Claude，2026-10-13）

- `deploy/docker-compose.yml` 给 `XC_TRUSTED_PROXIES` 加了默认值：本机回环和内网网段。面板端口只绑 `127.0.0.1`，能连进来的只有本机或容器里的反向代理，信任这些地址的转发头是安全的。`.env` 里写了这个变量时用 `.env` 的。
- 部署后代理重连，`whoami` 和连接来源会拿到真实公网 IP，curl 安装的、网卡上只有内网地址的服务器就能查到国家。
- hosts 包在 Windows 上失败的三个用例是换行符和本地环境的问题，CI 在 Linux 上跑。
