import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

// 开发时把 /api 代理到本地 Go 服务（go run ./cmd/ipan），
// 生产环境由 Go 服务直接托管 dist 目录。
export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // naive-ui 全量引入较大，关闭体积告警
    chunkSizeWarningLimit: 2048,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.IPAN_DEV_API || "http://127.0.0.1:8080",
        changeOrigin: true,
      },
    },
  },
});
