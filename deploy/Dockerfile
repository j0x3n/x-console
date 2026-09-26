# 构建前端和服务端，产出一个只含二进制和静态文件的镜像。
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS server
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X github.com/j0x3n/x-console/backend/internal/server/core.Version=${VERSION}" -o /out/x-console-server ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 xconsole
COPY --from=server /out/x-console-server /usr/local/bin/x-console-server
COPY --from=web /src/web/dist /srv/web
ENV XC_ADDR=0.0.0.0:8080 XC_DATA_DIR=/data XC_WEB_DIR=/srv/web
USER xconsole
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["x-console-server"]
