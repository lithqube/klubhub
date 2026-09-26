<script setup lang="ts">
import { Sun, Moon, Monitor } from 'lucide-vue-next'
import { useOnline } from '@vueuse/core'

/** KhMobileHeader — mobile brand bar + theme toggle shared by every KlubHub product. */
const props = withDefaults(defineProps<{ brand: string, status?: string }>(), {
  status: 'STATUS: SYNCED',
})

const { mode, setTheme } = useTheme()

// Real connectivity: the header status is a promise to someone backstage.
const online = useOnline()
const statusText = computed(() => (online.value ? props.status : 'OFFLINE'))

const opts = [
  { id: 'dark',   icon: Moon    },
  { id: 'system', icon: Monitor },
  { id: 'light',  icon: Sun     },
] as const
</script>

<template>
  <header
    class="lg:hidden flex-shrink-0 page-header"
    :aria-label="`${props.brand} brand bar`"
  >
    <!-- Brand -->
    <div>
      <div style="font-family:var(--font-command);font-size:13px;font-weight:800;color:var(--color-on-surface);text-transform:uppercase;letter-spacing:-.02em;line-height:1;">
        {{ props.brand }}
      </div>
      <div style="display:flex;align-items:center;gap:5px;margin-top:3px;">
        <div class="pulse-dot" aria-hidden="true" />
        <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);">
          {{ statusText }}
        </span>
      </div>
    </div>

    <!-- 3-way theme toggle -->
    <div class="theme-seg" style="gap:2px;">
      <button
        v-for="opt in opts"
        :key="opt.id"
        class="theme-opt hit-44"
        :class="mode === opt.id ? 'active' : ''"
        :aria-pressed="mode === opt.id"
        style="width:32px;height:26px;"
        @click="setTheme(opt.id)"
      >
        <component :is="opt.icon" style="width:11px;height:11px;" aria-hidden="true" />
      </button>
    </div>
  </header>
</template>
