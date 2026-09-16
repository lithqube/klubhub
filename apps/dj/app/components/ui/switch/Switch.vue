<script setup lang="ts">
import { cn } from '@/lib/utils'
import { SwitchRoot, SwitchThumb } from 'radix-vue'

interface Props {
  modelValue?: boolean
  disabled?: boolean
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: false,
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()
</script>

<template>
  <SwitchRoot
    :checked="modelValue"
    :disabled="disabled"
    :class="cn(
      'peer inline-flex w-8 h-[18px] shrink-0 cursor-pointer items-center rounded-none border transition-colors duration-200',
      'border-primary/20 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary',
      'disabled:cursor-not-allowed disabled:opacity-50',
      'data-[state=checked]:bg-primary/20 data-[state=checked]:border-primary data-[state=checked]:shadow-[0_0_10px_rgba(150,248,255,0.2)]',
      'data-[state=unchecked]:bg-surface-container-high',
      props.class
    )"
    @update:checked="emit('update:modelValue', $event)"
  >
    <SwitchThumb
      :class="cn(
        'pointer-events-none block w-3 h-3 rounded-none ring-0 transition-transform duration-200',
        'data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0.5',
        'data-[state=checked]:bg-primary data-[state=unchecked]:bg-tertiary'
      )"
    />
  </SwitchRoot>
</template>
