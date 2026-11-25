import { markSectionsShown, revealSections } from '../utils/motion'

export default defineNuxtPlugin((nuxtApp) => {
  nuxtApp.hooks.hookOnce('app:suspense:resolve', () => {
    // Content already on screen stays put; fading it again would flash.
    markSectionsShown()
    // Pages swap loading placeholders for data after navigation, so watch for
    // new sections instead of running once per page. The callback runs before
    // paint, and revealSections skips what it has already handled.
    new MutationObserver(() => revealSections()).observe(document.body, { childList: true, subtree: true })
  })
})
