import tailwindcss from '@tailwindcss/vite';
import { defineNuxtConfig } from 'nuxt/config';

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  workspaceDir: '../../',
  modules: ['@pinia/nuxt', 'shadcn-nuxt'],
  shadcn: {
    prefix: '',
    componentDir: './app/components/ui',
  },
  devtools: { enabled: true },
  devServer: {
    host: 'localhost',
    port: 4200,
  },
  typescript: {
    // Type checking handled separately via `pnpm nx typecheck @dev/dj`
    // Keeping typeCheck off here avoids pre-existing errors blocking the build
    typeCheck: false,
    tsConfig: {
      extends: '../../../tsconfig.base.json', // Nuxt copies this string as-is to the `./.nuxt/tsconfig.json`, therefore it needs to be relative to that directory
    },
  },
  imports: {
    autoImport: true,
  },
  css: ['~/assets/css/styles.css'],
  vite: {
    plugins: [tailwindcss()],
  },
  // Proxy /api/v1/* to the Go backend only when NUXT_PUBLIC_API_BASE is explicitly set.
  // Without it, Nitro serves the mock handlers in server/api/v1/ for local development.
  routeRules: process.env.NUXT_PUBLIC_API_BASE
    ? {
        '/api/v1/**': {
          proxy: process.env.NUXT_PUBLIC_API_BASE + '/api/v1/**',
        },
      }
    : {},
  nitro: process.env.NUXT_PUBLIC_API_BASE
    ? {
        devProxy: {
          '/api/v1': {
            target: process.env.NUXT_PUBLIC_API_BASE,
            changeOrigin: true,
          },
        },
      }
    : {},
}) as any;
