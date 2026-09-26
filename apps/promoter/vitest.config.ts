/// <reference types='vitest' />
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import path from 'path';

export default defineConfig(() => ({
  root: import.meta.dirname,
  cacheDir: '../../node_modules/.vite/apps/promoter',
  plugins: [vue()],
  resolve: {
    alias: {
      '~': path.resolve(import.meta.dirname, 'app'),
      '@': path.resolve(import.meta.dirname, 'app'),
      '#kui': path.resolve(import.meta.dirname, '../../libs/ui/app'),
    },
  },
  test: {
    name: '@dev/promoter',
    watch: false,
    globals: true,
    environment: 'jsdom',
    include: ['{app,server,tests}/**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts,jsx,tsx}'],
    reporters: ['default'],
    coverage: {
      reportsDirectory: './test-output/vitest/coverage',
      provider: 'v8' as const,
    },
  },
}));
