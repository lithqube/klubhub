<script setup lang="ts">
const route = useRoute()
await navigateTo(`/events/${route.params.id}/details`, { replace: true })
</script>

<template>
  <div />
</template>
