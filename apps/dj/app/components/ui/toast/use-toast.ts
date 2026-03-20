import { ref, computed } from 'vue'

export interface Toast {
  id: string
  title?: string
  description?: string
  variant?: 'default' | 'destructive'
  duration?: number
}

const toasts = ref<Toast[]>([])

let toastId = 0

export function useToast() {
  function toast(options: Omit<Toast, 'id'>) {
    const id = String(++toastId)
    const newToast: Toast = {
      id,
      duration: 5000,
      variant: 'default',
      ...options,
    }
    toasts.value.push(newToast)

    if (newToast.duration && newToast.duration > 0) {
      setTimeout(() => {
        dismiss(id)
      }, newToast.duration)
    }

    return id
  }

  function dismiss(id: string) {
    const index = toasts.value.findIndex((t) => t.id === id)
    if (index !== -1) {
      toasts.value.splice(index, 1)
    }
  }

  function dismissAll() {
    toasts.value = []
  }

  return {
    toasts: computed(() => toasts.value),
    toast,
    dismiss,
    dismissAll,
  }
}
