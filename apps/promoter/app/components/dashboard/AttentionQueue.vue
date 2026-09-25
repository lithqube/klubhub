<script setup lang="ts">
import { AlertTriangle, Clock, EyeOff, KeyRound, PencilLine, TimerOff } from 'lucide-vue-next'
import type { AttentionItem, AttentionKind } from '~/utils/attention'
import { collapseQueue } from '~/utils/attention'

const props = defineProps<{ items: AttentionItem[] }>()
const view = computed(() => collapseQueue(props.items))
const ICON = { conflict: AlertTriangle, untimed: TimerOff, goes_live: Clock, reveal: EyeOff, draft: PencilLine, security: KeyRound }
const MORE: Record<AttentionKind, string> = {
  conflict: 'CONFLICTS', untimed: 'UNTIMED', goes_live: 'GOING LIVE', reveal: 'REVEALS', draft: 'DRAFTS', security: '',
}
</script>

<template>
  <section aria-labelledby="attention-h">
    <h2 id="attention-h" class="section-lbl" style="margin:0 0 8px;">NEEDS YOUR ATTENTION · {{ items.length }}</h2>
    <p v-if="!items.length" class="glass accent-bar-ready" style="padding:14px 16px;margin:0;font-size:13px;">
      ALL CLEAR. Nothing needs you right now.
    </p>
    <ul v-else style="list-style:none;margin:0;padding:0;display:grid;gap:6px;">
      <li v-for="(i, n) in view.shown" :key="`${i.kind}-${i.to}-${n}`">
        <NuxtLink :to="i.to" class="glass attention-row" :class="`accent-bar-${i.tone}`">
          <component :is="ICON[i.kind]" :size="16" aria-hidden="true" :style="i.tone === 'failed' ? 'color:var(--color-error)' : 'color:var(--color-on-surface-variant)'" />
          <span class="data-frag" style="font-size:8px;" :style="i.tone === 'failed' ? 'color:var(--color-error)' : ''">{{ i.label }}</span>
          <span style="min-width:0;">
            <strong style="text-transform:uppercase;">{{ i.title }}</strong>
            <span style="color:var(--color-on-surface-variant);"> · {{ i.detail }}</span>
          </span>
          <span class="attention-cta">{{ i.action }} →</span>
        </NuxtLink>
      </li>
      <li v-for="(n, kind) in view.more" :key="`more-${kind}`" class="data-frag" style="font-size:9px;padding:4px 0;">
        +{{ n }} MORE {{ MORE[kind] }} · <NuxtLink :to="kind === 'draft' ? '/events?view=drafts' : '/events'" style="color:var(--color-primary);">SEE ALL</NuxtLink>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.attention-row {
  display: grid;
  grid-template-columns: 16px 84px minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
  padding: 10px 14px;
  min-height: 52px;
  text-decoration: none;
  color: inherit;
  font-size: 13px;
}
.attention-row:hover, .attention-row:focus-visible { border-color: var(--color-primary); }
.attention-cta {
  font-family: var(--font-terminal);
  font-size: 10px;
  letter-spacing: .08em;
  color: var(--color-primary);
  white-space: nowrap;
}
@media (max-width: 639px) {
  .attention-row { grid-template-columns: 16px minmax(0, 1fr) auto; }
  .attention-row > .data-frag { display: none; }
}
</style>
