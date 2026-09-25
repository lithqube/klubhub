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

async function handleAuthorize() {
  const url = await store.initiateOAuth()
  if (url) {
    window.location.href = url
  }
}
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">

    <!-- Page header — HUD design spec -->
    <SocialPageHeader
      :account="account"
      @disconnect="() => account && store.disconnectAccount(account!.id)"
      @new-post="handleAddToQueue"
    />

    <!-- Not-connected banner -->
    <div
      v-if="!account || account.status === 'disconnected'"
      style="margin:0 20px;margin-top:14px;padding:12px 16px;border:1px dashed color-mix(in srgb, var(--color-primary) 20%, transparent);display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;"
      class="glass"
    >
      <div>
        <div style="font-family:var(--font-command);font-size:11px;font-weight:600;color:var(--color-on-surface);text-transform:uppercase;letter-spacing:-.02em;">
          CONNECT INSTAGRAM
        </div>
        <div style="font-family:var(--font-data);font-size:12px;color:var(--color-tertiary);line-height:1.5;margin-top:2px;">
          Authorize to enable direct publishing. Queue works offline.
        </div>
      </div>
      <button class="btn-hud btn-hud-cta btn-hud-sm" style="padding:0 14px;" @click="handleAuthorize">
        AUTHORIZE →
      </button>
    </div>

    <!-- Token expiry warning -->
    <div
      v-else-if="account && account.tokenExpiry && isTokenExpiringSoon(account.tokenExpiry)"
      class="border border-dashed border-status-archived/30 bg-status-archived/5"
      style="margin:0 20px;margin-top:14px;padding:12px 16px;display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;"
    >
      <span class="font-terminal tracking-terminal text-status-archived" style="font-size:8px;letter-spacing:.06em;text-transform:uppercase;">
        INSTAGRAM TOKEN EXPIRING SOON — RE-AUTHORIZE
      </span>
      <button class="btn-hud btn-hud-ghost btn-hud-sm border-status-archived/30 text-status-archived" @click="handleAuthorize">
        RE-AUTHORIZE
      </button>
    </div>

    <!-- Main tabs content area -->
    <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;padding:0 20px;padding-top:14px;" class="social-tabs-wrapper">
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
