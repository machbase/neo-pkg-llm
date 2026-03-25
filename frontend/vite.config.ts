import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { viteSingleFile } from "vite-plugin-singlefile";

export default defineConfig({
    plugins: [react(), viteSingleFile()],
    build: {
        outDir: "dist",
        cssCodeSplit: false,
        assetsInlineLimit: 100000000,
        rollupOptions: {
            input: "index.html",
            output: {
                inlineDynamicImports: true,
            },
        },
    },
    server: {
        host: true,
        port: 5173,
        proxy: {
            "/api": "http://192.168.0.87:8884",
        },
    },
});
