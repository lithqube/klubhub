<script setup lang="ts">
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'
import { Check } from 'lucide-vue-next'

const badgeVariants = cva(
  'inline-flex items-center gap-1 px-2 py-0.5 font-terminal tracking-terminal text-xs uppercase font-medium rounded-none',
  {
    variants: {
      variant: {
        // OUTLINE PILLS (ghost-border, no fill)
        ready:       'ghost-border text-primary',
        draft:       'ghost-border text-secondary',
        archived:    'ghost-border text-tertiary',
        confirmed:   'ghost-border text-on-surface',
        'soft-hold': 'ghost-border text-on-surface',

        // FILLED PILLS
        failed:              'bg-error text-on-surface',
        scheduled:           'bg-primary text-on-primary',
        published:           'bg-surface-container-high text-tertiary',
        'awaiting-deposit':  'bg-error text-on-surface',
        nominal:             'bg-primary text-on-primary',

        // NON-PILL: left bar + text
        'paid-full': 'accent-bar-published text-on-surface pl-3 border-0 outline-none',
        settled:     'accent-bar-published text-on-surface pl-3 border-0 outline-none',

        // TEXT ONLY
        pending: 'text-error font-semibold border-0 outline-none px-0 py-0',

        // Default fallback
        default: 'ghost-border text-on-surface-variant',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  }
)

type BadgeVariants = VariantProps<typeof badgeVariants>

interface Props {
  variant?: BadgeVariants['variant']
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
})
</script>

<template>
  <span :class="cn(badgeVariants({ variant }), props.class)">
    <!-- Dot indicator for scheduled -->
    <span
      v-if="variant === 'scheduled'"
      class="inline-block w-1.5 h-1.5 rounded-none bg-on-primary shrink-0"
    />
    <slot />
    <!-- Checkmark for published -->
    <Check v-if="variant === 'published'" class="w-3 h-3 shrink-0" />
  </span>
</template>
