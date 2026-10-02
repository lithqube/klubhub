<script setup lang="ts">
// RiderAttachmentInlineCard — wrapper around RiderAttachmentCard for
// use inside GigFormDialog. Same behavior, but the open-editor button
// navigates to /rider?gig=<id> rather than emitting.

import type { Gig } from '~/types/gig'
import RiderAttachmentCard from './RiderAttachmentCard.vue'

const props = defineProps<{ gig: Gig }>()

async function onOpenEditor(gigId: string): Promise<void> {
  await navigateTo({ path: '/rider', query: { gig: gigId } })
}
</script>

<template>
  <div data-testid="gig-form-rider-card">
    <label class="section-lbl" style="display:block;margin-bottom:6px;">RIDER</label>
    <RiderAttachmentCard :gig="props.gig" @open-editor="onOpenEditor" />
  </div>
</template>