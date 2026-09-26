/// <reference types='vitest' />
import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import path from 'path';

export default defineConfig(() => ({
  root: import.meta.dirname,
  cacheDir: '../../node_modules/.vite/libs/ui',
  plugins: [vue()],
  resolve: {
    alias: {
      '#kui': path.resolve(import.meta.dirname, 'app'),
    },
  },
  test: {
    name: '@dev/ui',
    watch: false,
    globals: true,
    environment: 'jsdom',
    include: ['app/**/*.{test,spec}.ts'],
    reporters: ['default'],
  },
}));
