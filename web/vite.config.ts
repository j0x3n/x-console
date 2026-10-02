import { defineConfig } from "vite";

// 开发时把 /api 转发到本地 Go 服务（默认 127.0.0.1:8080），包括 WebSocket。
export default defineConfig({
  // 版本号和构建时间写进前端（B79）。部署时 Dockerfile 传 XC_VERSION。
  define: {
    __XC_VERSION__: JSON.stringify(process.env.XC_VERSION || "dev"),
    __XC_BUILT_AT__: JSON.stringify(new Date().toISOString()),
  },
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
  build: {
    rollupOptions: {
      output: {
        // 第三方库单独打包（B6）。它们很少变，浏览器能一直用缓存；主包也不超过 500 kB。
        manualChunks(id) {
          if (!id.includes("node_modules")) return undefined;
          if (/node_modules\/(react|react-dom|scheduler)\//.test(id))
            return "react";
          if (
            /node_modules\/(react-router|@tanstack|zustand|openapi-fetch)\//.test(
              id,
            )
          )
            return "vendor";
          // 图标合成一个包，免得拆成几十个很小的文件。
          if (/node_modules\/lucide-react\//.test(id)) return "icons";
          return undefined;
        },
      },
    },
  },
  test: {
    environment: "node",
    maxWorkers: 1,
    testTimeout: 30_000,
    setupFiles: ["./src/test/setup.ts"],
  },
});
