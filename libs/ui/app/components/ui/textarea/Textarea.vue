<script setup lang="ts">
import { cn } from '#kui/lib/utils'
import { useVModel } from '@vueuse/core'

const props = defineProps<{
  defaultValue?: string
  modelValue?: string
  class?: string
  placeholder?: string
  disabled?: boolean
  readonly?: boolean
  rows?: number
}>()

const emits = defineEmits<{
  (e: 'update:modelValue', payload: string): void
}>()

const modelValue = useVModel(props, 'modelValue', emits, {
  passive: true,
  defaultValue: props.defaultValue,
})
</script>

<template>
  <textarea
    v-model="modelValue"
    :placeholder="placeholder"
    :disabled="disabled"
    :readonly="readonly"
    :rows="rows ?? 4"
    :class="cn(
      'flex w-full rounded-none bg-surface-container-high px-3 py-2 text-sm text-on-surface',
      'border-0 border-l-2 border-transparent',
      'focus-visible:outline-none focus-visible:border-primary focus-visible:ring-0',
      'placeholder:text-tertiary placeholder:font-terminal placeholder:uppercase placeholder:tracking-terminal placeholder:text-[10px]',
      'disabled:cursor-not-allowed disabled:opacity-50',
      'transition-colors duration-150 resize-none',
      props.class,
    )"
  />
</template>
