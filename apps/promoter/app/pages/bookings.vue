<script setup lang="ts">
useHead({ title: 'Bookings' })
</script>

<template>
  <PhasePlaceholder
    title="BOOKINGS"
    phase="P4"
    summary="Offers, contracts and advancing, shared with artists through the KlubHub booking exchange."
    :features="[
      'Holds and offers with deal types',
      'Contracts and riders',
      'Advancing with a shareable form',
      'Guest-list allocation and set times synced to the DJ app',
    ]"
  />
</template>
