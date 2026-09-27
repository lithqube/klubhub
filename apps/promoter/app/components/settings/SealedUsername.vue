<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useOrgStore } from '~/stores/org'
import { useSessionStore } from '~/stores/session'

/**
 * A hidden, read-only "username" next to a sealed passphrase field, so a
 * password manager files the passphrase as its own entry ("<email> ·
 * KlubHub sealed passphrase") instead of overwriting the login password.
 * /auth/me carries no email today: the collective's name stands in until it does.
 */
const { me } = storeToRefs(useSessionStore())
const { org } = storeToRefs(useOrgStore())
const value = computed(() => `${me.value?.email || org.value?.name || 'KlubHub'} · KlubHub sealed passphrase`)
</script>

<template>
  <input class="pm-user" type="text" name="username" autocomplete="username" :value="value" readonly tabindex="-1" aria-hidden="true" data-testid="sealed-pm-username">
</template>

<style scoped>
/* Not display:none: password managers skip fields that are not rendered. */
.pm-user {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  border: 0;
  opacity: 0;
  overflow: hidden;
  clip: rect(0 0 0 0);
  pointer-events: none;
}
</style>
