import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'

const API_BASE_URL = process.env.API_BASE_URL || 'http://localhost:1524'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    watch: {
      usePolling: true
    },
    proxy: {
      '^\/health$': {
        target: API_BASE_URL,
        changeOrigin: true,
      },
      '^\/node$': {
        target: API_BASE_URL,
        changeOrigin: true,
      },
      '^\/connect$': {
        target: API_BASE_URL,
        changeOrigin: true,
      },
      '/devices': {
        target: API_BASE_URL,
        changeOrigin: true,
      },
      '/proxy': {
        target: API_BASE_URL,
        changeOrigin: true,
      },
      '/policy': {
        target: API_BASE_URL,
        changeOrigin: true,
      },
    }
  }
})
