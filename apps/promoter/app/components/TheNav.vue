<script setup lang="ts">
import { useSessionStore } from '~/stores/session'
import { PROMOTER_NAV } from '~/utils/nav'

const session = useSessionStore()
const { items: attention, hasConflict, load } = useAttention()
// The counter is client-only: SSR renders the nav before page data loads.
const mounted = ref(false)
onMounted(() => {
  mounted.value = true
  load().catch(() => {})
})

const items = computed(() => PROMOTER_NAV.map(i => (i.to === '/' && mounted.value && attention.value.length
  ? { ...i, count: attention.value.length, countTone: hasConflict.value ? 'error' as const : 'default' as const }
  : i)))

async function logout() {
  await session.logout().catch(() => {})
  await navigateTo('/login')
}
</script>

<template>
  <KhSideNav brand="KlubHub Promoter" :items="items" show-logout @logout="logout" />
</template>
