import { defineConfig } from "vite";

// 开发时把 /api 转发到本地 Go 服务（默认 127.0.0.1:8080），包括 WebSocket。
export default defineConfig({
  server: {
    host: "127.0.0.1",
    proxy: {
      "/api": {
        target: process.env.XC_API ?? "http://127.0.0.1:8080",
        ws: true,
        changeOrigin: false,
      },
    },
  },
  test: {
    environment: "node",
  },
});
