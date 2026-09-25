import { useSessionStore } from '~/stores/session'

/** Pages reachable without a session (they carry `public: true` meta). */
export default defineNuxtRouteMiddleware(async (to) => {
  if (to.meta.public) return
  const session = useSessionStore()
  if (!session.loaded) await session.fetchMe()
  if (!session.isAuthenticated) {
    return navigateTo({ path: '/login', query: to.fullPath !== '/' ? { next: to.fullPath } : {} })
  }
})
