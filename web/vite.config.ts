import { defineConfig } from 'vite-plus'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [tanstackRouter({ target: 'react', autoCodeSplitting: true }), react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  fmt: {
    singleQuote: true,
    jsxSingleQuote: true,
    semi: false,
    ignorePatterns: ['**/dist/**', '**/node_modules/**', '**/routeTree.gen.ts'],
  },
  lint: {
    ignorePatterns: ['**/dist/**', '**/node_modules/**', '**/routeTree.gen.ts'],
  },
  server: {
    port: 4173,
    proxy: {
      '/api': 'http://localhost:3825',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
