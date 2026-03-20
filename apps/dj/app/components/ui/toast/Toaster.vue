<script setup lang="ts">
import { useToast } from './use-toast'
import Toast from './Toast.vue'

const { toasts, dismiss } = useToast()
</script>

<template>
  <div
    class="fixed bottom-4 right-4 z-[100] flex max-h-screen w-full max-w-sm flex-col gap-2"
    aria-live="polite"
    aria-label="Notifications"
  >
    <Toast
      v-for="toast in toasts"
      :key="toast.id"
      :toast="toast"
      @dismiss="dismiss"
    />
  </div>
</template>
