<script setup lang="ts">
import { cn } from '@/lib/utils'
import { useVModel } from '@vueuse/core'

const props = defineProps<{
  defaultValue?: string | number
  modelValue?: string | number
  class?: string
  type?: string
  placeholder?: string
  disabled?: boolean
  readonly?: boolean
}>()

const emits = defineEmits<{
  (e: 'update:modelValue', payload: string | number): void
}>()

const modelValue = useVModel(props, 'modelValue', emits, {
  passive: true,
  defaultValue: props.defaultValue,
})
</script>

<template>
  <input
    v-model="modelValue"
    :type="type ?? 'text'"
    :placeholder="placeholder"
    :disabled="disabled"
    :readonly="readonly"
    :class="cn(
      'flex h-9 w-full rounded-none bg-surface-container-high px-3 py-1 text-sm text-on-surface',
      'border-0 border-l-2 border-transparent',
      'focus-visible:outline-none focus-visible:border-primary focus-visible:ring-0',
      'placeholder:text-tertiary',
      'disabled:cursor-not-allowed disabled:opacity-50',
      'transition-colors duration-150',
      props.class,
    )"
  />
</template>
