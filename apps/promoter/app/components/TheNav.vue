<script setup lang="ts">
import { useSessionStore } from '~/stores/session'
import { PROMOTER_NAV } from '~/utils/nav'

const session = useSessionStore()

async function logout() {
  await session.logout().catch(() => {})
  await navigateTo('/login')
}
</script>

<template>
  <KhSideNav brand="KlubHub Promoter" :items="PROMOTER_NAV" show-logout @logout="logout" />
</template>
