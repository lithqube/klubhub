<script setup lang="ts">
// Link that opens a stored receipt in a new tab. The plain URL answers with
// `Content-Disposition: attachment`, so a PDF downloads and an image opens
// or downloads depending on the browser; thumbnails use the `?inline=1` URL
// instead. In the browser demo the URL is a data: URL, which browsers refuse
// to open in a new tab, so there the link downloads the file.
import { computed } from 'vue'
import { entryAttachmentUrl } from '../../stores/earnings'
import type { EntryAttachment } from '../../types/finance'

const props = defineProps<{ attachment: EntryAttachment }>()
const href = computed(() => entryAttachmentUrl(props.attachment.entry_id, props.attachment.id))
</script>

<template>
  <a
    class="rl"
    :href="href"
    target="_blank"
    rel="noopener"
    :download="href.startsWith('data:') ? attachment.filename : undefined"
  >{{ attachment.filename }}<span class="sr-only"> (opens in a new tab)</span></a>
</template>

<style scoped>
.rl { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: var(--font-data); font-size: 13px; color: var(--color-on-surface); text-decoration: underline; text-decoration-style: dashed; text-underline-offset: 3px; }
.rl:hover { color: var(--color-primary); }
.rl:focus-visible { outline: 2px solid var(--color-primary); outline-offset: 2px; }
</style>
