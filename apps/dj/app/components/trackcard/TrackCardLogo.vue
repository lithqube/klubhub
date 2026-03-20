<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps({
  logoPath: {
    type: [String, null] as unknown as () => string | null,
    default: null,
  },
  logoPosition: {
    type: String as () =>
      | 'top-left'
      | 'top-right'
      | 'bottom-left'
      | 'bottom-right',
    default: 'top-left',
  },
});

const logoStyle = computed(() => {
  if (!props.logoPath) return { display: 'none' };

  const style: Record<string, string> = {
    position: 'absolute',
    width: '120px',
    height: 'auto',
    objectFit: 'contain',
  };

  switch (props.logoPosition) {
    case 'top-left':
      style.top = '20px';
      style.left = '20px';
      break;
    case 'top-right':
      style.top = '20px';
      style.right = '20px';
      break;
    case 'bottom-left':
      style.bottom = '20px';
      style.left = '20px';
      break;
    case 'bottom-right':
      style.bottom = '20px';
      style.right = '20px';
      break;
  }

  return style;
});
</script>

<template>
  <img
    v-if="logoPath"
    :src="logoPath"
    :style="logoStyle"
    alt="Logo"
  />
</template>
