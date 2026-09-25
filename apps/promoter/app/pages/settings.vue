<script setup lang="ts">
useHead({ title: 'Settings' })
</script>

<template>
  <PhasePlaceholder
    title="SETTINGS"
    phase="P0"
    summary="Organisation, team and security settings."
    :features="[
      'Organisation profile, timezone and currency',
      'Team members and roles',
      'Door devices and PINs',
      'Data retention and encryption status',
    ]"
  />
</template>
