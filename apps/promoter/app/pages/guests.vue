<script setup lang="ts">
useHead({ title: 'Guests' })
</script>

<template>
  <PhasePlaceholder
    title="GUESTS"
    phase="P2"
    summary="Guest lists with per-submitter quotas, standing lists and imports from your ticketing platform."
    :features="[
      'List types with entry terms and cutoff',
      'Per-DJ allocations synced from bookings',
      'CSV import from RA, Shotgun and DICE',
      'Automatic personal-data purge after the event',
    ]"
  />
</template>
