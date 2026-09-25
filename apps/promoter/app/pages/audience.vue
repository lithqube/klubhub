<script setup lang="ts">
useHead({ title: 'Audience' })
</script>

<template>
  <PhasePlaceholder
    title="AUDIENCE"
    phase="P3"
    summary="A consent-first audience of followers, past attendees and notify-me sign-ups."
    :features="[
      'Consent record for every contact',
      'Segments by event, city and genre',
      'Provider-hosted opt-in and unsubscribe',
      'Export and deletion on request',
    ]"
  />
</template>
