<script setup lang="ts">
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'
import { Primitive, type PrimitiveProps } from 'radix-vue'
import { computed } from 'vue'

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap font-terminal tracking-terminal text-[9px] font-semibold uppercase transition-all duration-200 disabled:pointer-events-none disabled:opacity-50 rounded-none focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary',
  {
    variants: {
      variant: {
        default: 'gradient-cta text-on-primary shadow-glow-primary hover:shadow-glow-primary-strong',
        secondary: 'bg-transparent ghost-border text-secondary hover:text-primary hover:luminous-threshold',
        violet: 'btn-hud-violet',
        destructive: 'bg-error-container text-error shadow-glow-error hover:shadow-glow-error',
        outline: 'bg-transparent ghost-border text-on-surface hover:text-primary hover:luminous-threshold',
        ghost: 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high',
        link: 'text-primary underline-offset-4 hover:underline',
      },
      size: {
        default: 'h-9 px-4 py-2',
        sm: 'h-8 px-3',
        lg: 'h-10 px-6',
        icon: 'h-9 w-9',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  }
)

type ButtonVariants = VariantProps<typeof buttonVariants>

interface Props extends PrimitiveProps {
  variant?: ButtonVariants['variant']
  size?: ButtonVariants['size']
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  as: 'button',
})

const delegatedProps = computed(() => {
  const { class: _, variant: __, size: ___, ...delegated } = props
  return delegated
})
</script>

<template>
  <Primitive
    v-bind="delegatedProps"
    :class="cn(buttonVariants({ variant, size }), props.class)"
  >
    <slot />
  </Primitive>
</template>
