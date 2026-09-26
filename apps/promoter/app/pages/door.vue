<script setup lang="ts">
useHead({ title: 'Door' })
</script>

<template>
  <PhasePlaceholder
    title="DOOR"
    phase="P2"
    summary="An offline-first door app for staff on a PIN, with live occupancy."
    :features="[
      'Name search and QR scan',
      'Partial arrivals, one-tap undo',
      'Live in/out occupancy vs capacity',
      'Works without signal, syncs after',
    ]"
  />
</template>
