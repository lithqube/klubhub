<script setup lang="ts">
/**
 * Slim, always-visible strip for the browser-only demo build (docs/DEMO.md).
 * Reset is two-step (arm, then confirm) like other destructive HUD actions.
 */
import { ExternalLink, RotateCcw } from 'lucide-vue-next'

const { reset } = useDemo()
const armed = ref(false)
let disarm: ReturnType<typeof setTimeout> | null = null

function onReset() {
  if (!armed.value) {
    armed.value = true
    disarm = setTimeout(() => { armed.value = false }, 4000)
    return
  }
  if (disarm) clearTimeout(disarm)
  reset()
  window.location.reload()
}

onBeforeUnmount(() => { if (disarm) clearTimeout(disarm) })
</script>

<template>
  <div class="demo-banner" role="region" aria-label="Demo mode" data-testid="demo-banner">
    <span class="demo-tag">DEMO</span>
    <span class="demo-copy">Your changes stay in this browser</span>
    <span class="demo-actions">
      <button
        type="button"
        class="demo-btn"
        :class="{ 'demo-btn-armed': armed }"
        data-testid="demo-reset"
        :aria-label="armed ? 'Confirm: reset the demo data' : 'Reset demo data'"
        @click="onReset"
      >
        <RotateCcw style="width:10px;height:10px;" aria-hidden="true" />
        {{ armed ? 'CONFIRM RESET?' : 'RESET DEMO' }}
      </button>
      <a class="demo-link" href="https://klubhub.io/" rel="noopener">
        klubhub.io
      </a>
      <a class="demo-link" href="https://github.com/lithqube/klubhub" rel="noopener" target="_blank">
        Get it on GitHub <ExternalLink style="width:9px;height:9px;" aria-hidden="true" />
      </a>
    </span>
  </div>
</template>

<style scoped>
.demo-banner {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 12px;
  padding: 5px 16px;
  flex-shrink: 0;
  background: color-mix(in srgb, var(--color-primary) 8%, var(--color-surface-container-low));
  border-bottom: 1px dashed color-mix(in srgb, var(--color-primary) 30%, transparent);
  font-family: var(--font-terminal);
  font-size: 9px;
  letter-spacing: .06em;
  text-transform: uppercase;
  color: var(--color-on-surface-variant);
}
.demo-tag {
  padding: 1px 6px;
  font-weight: 700;
  color: var(--color-on-primary);
  background: var(--color-primary);
}
.demo-copy { flex: 1 1 auto; min-width: 0; }
.demo-actions { display: inline-flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.demo-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-height: 24px;
  padding: 0 8px;
  font: inherit;
  font-weight: 600;
  letter-spacing: inherit;
  color: var(--color-on-surface-variant);
  background: transparent;
  border: 1px dashed color-mix(in srgb, var(--color-outline) 90%, transparent);
  cursor: pointer;
}
.demo-btn:hover, .demo-btn:focus-visible { color: var(--color-primary); border-color: color-mix(in srgb, var(--color-primary) 50%, transparent); }
.demo-btn-armed { color: var(--color-error); border-color: var(--color-error); }
.demo-link {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  min-height: 24px;
  color: var(--color-primary);
  text-decoration: none;
}
.demo-link:hover, .demo-link:focus-visible { text-decoration: underline; }
@media (max-width: 420px) {
  .demo-copy { flex-basis: calc(100% - 60px); }
}
</style>
