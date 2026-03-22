<script setup lang="ts">
import { useEpkStore } from '~/stores/epk'

useHead({ title: 'EPK | KlubHub DJ' })

const store = useEpkStore()

onMounted(async () => {
  await Promise.all([
    store.loadFromApi(),
    store.loadExports(),
  ])
})
</script>

<template>
  <div class="flex flex-col gap-8 p-4 lg:p-8 max-w-3xl mx-auto">
    <EpkPageHeader />
    <EpkBioSection />
    <EpkPhotosSection />
    <EpkTechRiderSection />
    <EpkStagePlotSection />
    <EpkGigHighlightsSection />
    <EpkPressQuotesSection />
    <EpkSocialLinksSection />
    <EpkContactInfoSection />
    <EpkExportPanel />
  </div>
</template>
