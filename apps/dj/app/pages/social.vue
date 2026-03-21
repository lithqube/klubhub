<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useSocialStore } from '~/stores/social'
import SocialPageHeader from '~/components/social/SocialPageHeader.vue'
import SocialTabs from '~/components/social/SocialTabs.vue'

useHead({ title: 'Social Scheduler — KlubHub DJ' })

const route = useRoute()
const store = useSocialStore()
const { posts, account, composePanelOpen, prefilledImageId } = storeToRefs(store)

function isTokenExpiringSoon(tokenExpiry: string): boolean {
  const expiry = new Date(tokenExpiry)
  const now = new Date()
  const sevenDays = 7 * 24 * 60 * 60 * 1000
  return expiry.getTime() - now.getTime() < sevenDays
}

onMounted(async () => {
  await Promise.all([store.loadPosts(), store.loadAccount()])
  const imageId = route.query.imageId
  if (imageId && typeof imageId === 'string') {
    store.openComposePanel(imageId)
    nextTick(() => {
      document.querySelector('[data-compose-panel]')?.scrollIntoView({ behavior: 'smooth' })
    })
  }
})

function handleAddToQueue() {
  store.openComposePanel()
}

function handleQuickExport() {
  store.openComposePanel()
}
</script>

<template>
  <div class="flex-1 flex flex-col overflow-hidden">

    <!-- Page header -->
    <SocialPageHeader :account="account" @disconnect="() => account && store.disconnectAccount(account!.id)" />

    <!-- Connection banner: no account or disconnected -->
    <div
      v-if="!account || account.status === 'disconnected'"
      class="mx-8 mt-4 px-5 py-3 border border-secondary/40 bg-secondary/10 flex items-center justify-between"
    >
      <div class="flex items-center gap-3">
        <span class="text-secondary font-terminal tracking-terminal text-xs uppercase">
          ● INSTAGRAM NOT CONNECTED
        </span>
        <span class="font-data text-xs text-on-surface-variant">
          Connect your Instagram account to start scheduling posts.
        </span>
      </div>
      <button class="ghost-border px-4 py-1.5 font-terminal tracking-terminal text-xs uppercase text-secondary border-secondary/40 hover:border-secondary transition-colors">
        CONNECT INSTAGRAM
      </button>
    </div>

    <!-- Token expiry warning banner -->
    <div
      v-else-if="account && account.tokenExpiry && isTokenExpiringSoon(account.tokenExpiry)"
      class="mx-8 mt-4 px-5 py-3 border border-amber-500/40 bg-amber-500/10 flex items-center justify-between"
    >
      <span class="font-terminal tracking-terminal text-xs text-amber-400 uppercase">
        ⚠ INSTAGRAM TOKEN EXPIRING SOON — RE-AUTHORIZE
      </span>
      <button class="ghost-border px-4 py-1.5 font-terminal tracking-terminal text-xs uppercase text-amber-400 border-amber-500/40 hover:border-amber-400 transition-colors">
        RE-AUTHORIZE
      </button>
    </div>

    <!-- Main tabs content area -->
    <div class="flex-1 flex flex-col overflow-hidden px-8 pt-4">
      <SocialTabs
        :posts="posts"
        :compose-panel-open="composePanelOpen"
        :prefilled-image-id="prefilledImageId"
        @add-to-queue="handleAddToQueue"
        @quick-export="handleQuickExport"
      />
    </div>

  </div>
</template>
