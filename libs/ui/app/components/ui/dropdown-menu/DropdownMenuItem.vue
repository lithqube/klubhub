<script setup lang="ts">
import { DropdownMenuItem, type DropdownMenuItemProps } from 'radix-vue'
import { cn } from '#kui/lib/utils'

interface Props extends DropdownMenuItemProps {
  class?: string
  inset?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  inset: false,
})
</script>

<template>
  <DropdownMenuItem
    v-bind="props"
    :class="cn(
      'relative flex cursor-pointer select-none items-center gap-2 px-2 py-1.5',
      'font-terminal tracking-terminal text-xs uppercase text-on-surface',
      'transition-colors duration-150',
      'focus:bg-surface-container-high focus:text-primary focus:outline-none',
      'data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
      inset && 'pl-8',
      props.class
    )"
  >
    <slot />
  </DropdownMenuItem>
</template>
