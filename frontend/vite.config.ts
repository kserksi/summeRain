import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { sri } from 'vite-plugin-sri3'
import fs from 'node:fs'
import path from 'node:path'

const hasCert = fs.existsSync(path.resolve(import.meta.dirname, 'localhost+1-key.pem'))

export default defineConfig({
  base: '/',
  plugins: [
    react(),
    tailwindcss(),
    sri(),
  ],
  resolve: {
    alias: { '@': path.resolve(import.meta.dirname, './src') },
  },
  server: {
    https: hasCert
      ? {
          key: fs.readFileSync(path.resolve(import.meta.dirname, 'localhost+1-key.pem')),
          cert: fs.readFileSync(path.resolve(import.meta.dirname, 'localhost+1.pem')),
        }
      : undefined,
    proxy: {
      '/api/': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        secure: false,
        timeout: 30000,
      },
      '/i/': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        secure: false,
        timeout: 60000,
      },
    },
  },
  build: {
    outDir: '../backend/web',
    emptyOutDir: true,
  },
})
