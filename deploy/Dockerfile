# 构建前端和服务端，产出一个只含二进制和静态文件的镜像。
# 前端和 Go 都在构建机的原生平台上编译，Go 交叉编译到目标架构，
# 所以构建 arm64 镜像时不需要模拟器，速度和 amd64 一样。
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS server
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X github.com/j0x3n/x-console/backend/internal/server/core.Version=${VERSION} -X github.com/j0x3n/x-console/backend/internal/server/core.BuiltAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /out/x-console-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /out/x-console-agent ./cmd/agent
# 面板发给别的机器安装的代理：四个平台各一份，放在 /out/agents/<系统>-<架构>/，旁边写 SHA256SUMS。
# 第一次编译后 Go 的编译缓存还在，后面几个平台很快。
RUN set -eu; \
    for target in linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
        os=${target%/*}; arch=${target#*/}; name=x-console-agent; \
        [ "$os" = windows ] && name=x-console-agent.exe; \
        mkdir -p /out/agents/$os-$arch; \
        CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
            -ldflags "-s -w -X main.Version=${VERSION}" \
            -o /out/agents/$os-$arch/$name ./cmd/agent; \
        (cd /out/agents/$os-$arch && sha256sum $name > SHA256SUMS); \
    done

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 xconsole \
    && mkdir /data && chown xconsole /data
COPY --from=server /out/x-console-server /usr/local/bin/x-console-server
COPY --from=server /out/x-console-agent /usr/local/bin/x-console-agent
COPY --from=server /out/agents /usr/share/x-console/agents
COPY --from=web /src/web/dist /srv/web
ENV XC_ADDR=0.0.0.0:8080 XC_DATA_DIR=/data XC_WEB_DIR=/srv/web XC_AGENTS_DIR=/usr/share/x-console/agents
USER xconsole
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
    CMD wget -qO- http://127.0.0.1:8080/api/v1/health >/dev/null || exit 1
ENTRYPOINT ["x-console-server"]
