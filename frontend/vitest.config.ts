import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// Minimal unit-test config, separate from vite.config.ts's build-oriented
// plugin list (compression etc. are not needed for tests). Shares the same
// '@' alias so components under test resolve imports identically to the app.
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  test: {
    environment: 'happy-dom',
    include: ['src/**/*.{spec,test}.ts']
  }
})
