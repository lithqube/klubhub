<script setup lang="ts">
import { cn } from '@/lib/utils'
import { AvatarRoot, AvatarImage, AvatarFallback } from 'radix-vue'

interface Props {
  src?: string
  alt?: string
  fallback?: string
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  alt: '',
  fallback: '??',
})
</script>

<template>
  <AvatarRoot
    :class="cn('relative flex h-9 w-9 shrink-0 overflow-hidden rounded-full', props.class)"
  >
    <AvatarImage
      v-if="src"
      :src="src"
      :alt="alt"
      class="aspect-square h-full w-full object-cover"
    />
    <AvatarFallback
      class="flex h-full w-full items-center justify-center bg-surface-container font-terminal tracking-terminal text-xs text-tertiary uppercase rounded-full"
    >
      {{ fallback }}
    </AvatarFallback>
  </AvatarRoot>
</template>
