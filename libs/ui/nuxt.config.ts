import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath } from 'node:url'
import { defineNuxtConfig } from 'nuxt/config'

// @dev/ui — the Kinetic HUD Nuxt layer shared by every KlubHub product.
//
// Apps consume it with `extends: ['../../libs/ui']` and import
// `#kui/assets/css/kinetic.css` at the top of their entry stylesheet.
//
// Inside a layer, `~` and `@` resolve to the *consuming* app, so every
// layer-internal import goes through the `#kui` alias instead.
const appDir = fileURLToPath(new URL('./app', import.meta.url))

export default defineNuxtConfig({
  alias: {
    '#kui': appDir,
  },
  modules: ['shadcn-nuxt'],
  shadcn: {
    prefix: '',
    componentDir: fileURLToPath(new URL('./app/components/ui', import.meta.url)),
  },
  vite: {
    plugins: [tailwindcss()],
  },
})
