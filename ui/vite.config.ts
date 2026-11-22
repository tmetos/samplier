import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The API port can be overridden (the demo script sets VITE_API_PORT).
const apiPort = process.env.VITE_API_PORT ?? '8080'
const apiTarget = `http://localhost:${apiPort}`

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: apiTarget, changeOrigin: true },
      '/ogc': { target: apiTarget, changeOrigin: true },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test-setup.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      include: ['src/lib/**/*.ts', 'src/hooks/**/*.ts'],
      thresholds: { statements: 80, branches: 80, functions: 80, lines: 80 },
    },
  },
})
