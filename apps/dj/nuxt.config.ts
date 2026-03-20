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
    typeCheck: true,
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
  // Proxy all /api/v1/* requests to the Go backend.
  // Production: NUXT_PUBLIC_API_BASE is set to http://api:8080 in docker-compose.yml
  // Dev: nitro.devProxy below handles the same path via localhost:8080
  routeRules: {
    '/api/v1/**': {
      proxy:
        (process.env.NUXT_PUBLIC_API_BASE ?? 'http://localhost:8080') +
        '/api/v1/**',
    },
  },
  nitro: {
    devProxy: {
      '/api/v1': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
}) as any;
