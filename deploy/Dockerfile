FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
ARG VERSION=dev
ENV XC_VERSION=$VERSION
RUN npm run build

FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/golang:1.26-alpine AS go-base
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
ENV CGO_ENABLED=0

FROM go-base AS server-compile
COPY backend/ ./
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w" -o /out/x-console-server ./cmd/server

FROM server-compile AS server
ARG VERSION=dev
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X github.com/j0x3n/x-console/backend/internal/server/core.Version=${VERSION} -X github.com/j0x3n/x-console/backend/internal/server/core.BuiltAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /out/x-console-server ./cmd/server

FROM go-base AS agent-compile
COPY backend/cmd/agent/ ./cmd/agent/
COPY backend/internal/agent/ ./internal/agent/
COPY backend/pkg/ ./pkg/
RUN set -eu; \
    for target in linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
        os=${target%/*}; arch=${target#*/}; name=x-console-agent; \
        [ "$os" = windows ] && name=x-console-agent.exe; \
        mkdir -p /out/agents/$os-$arch; \
        gui=; [ "$os" = windows ] && gui="-H=windowsgui"; \
        GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w $gui" \
            -o /out/agents/$os-$arch/$name ./cmd/agent; \
    done

FROM agent-compile AS agents
ARG VERSION=dev
RUN set -eu; \
    for target in linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
        os=${target%/*}; arch=${target#*/}; name=x-console-agent; \
        [ "$os" = windows ] && name=x-console-agent.exe; \
        gui=; [ "$os" = windows ] && gui="-H=windowsgui"; \
        GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w $gui -X main.Version=${VERSION}" \
            -o /out/agents/$os-$arch/$name ./cmd/agent; \
        (cd /out/agents/$os-$arch && sha256sum $name > SHA256SUMS); \
    done
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN cp /out/agents/$TARGETOS-$TARGETARCH/x-console-agent /out/x-console-agent

FROM mirror.gcr.io/library/alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 xconsole \
    && mkdir /data && chown xconsole /data
COPY --from=server /out/x-console-server /usr/local/bin/x-console-server
COPY --from=agents /out/x-console-agent /usr/local/bin/x-console-agent
COPY --from=agents /out/agents /usr/share/x-console/agents
COPY --from=web /src/web/dist /srv/web
ENV XC_ADDR=0.0.0.0:8080 XC_DATA_DIR=/data XC_WEB_DIR=/srv/web XC_AGENTS_DIR=/usr/share/x-console/agents
USER xconsole
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
    CMD wget -qO- http://127.0.0.1:8080/api/v1/health >/dev/null || exit 1
ENTRYPOINT ["x-console-server"]
