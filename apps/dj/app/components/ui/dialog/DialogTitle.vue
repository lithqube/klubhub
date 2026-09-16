<script setup lang="ts">
import { cn } from '@/lib/utils'
import { DialogTitle, type DialogTitleProps } from 'radix-vue'
import { computed } from 'vue'

const props = defineProps<DialogTitleProps & { class?: string }>()

const delegatedProps = computed(() => {
  const { class: _, ...delegated } = props
  return delegated
})
</script>

<template>
  <DialogTitle
    v-bind="delegatedProps"
    :class="cn('font-command tracking-command text-lg font-bold text-on-surface uppercase', props.class)"
  >
    <slot />
  </DialogTitle>
</template>
