<script setup lang="ts">
import { signInAgainRoute } from '~/utils/privacy'

/**
 * Why an owner key action cannot run yet: no second factor (link to 2FA
 * setup) or the sign-in is older than 15 minutes (SIGN IN AGAIN, then
 * straight back to `next`). One message (a note, not an alert: it is a
 * precondition shown on arrival, not the result of an action); the caller
 * decides when and makes sure only one shows at a time.
 */
const props = defineProps<{ kind: 'mfa' | 'stale', next: string, action?: string }>()
const route = computed(() => signInAgainRoute(props.next, 'sealed'))
/** 2FA setup, then straight back here (the security page offers CONTINUE → next). */
const mfaRoute = computed(() => ({ path: '/account/security', query: { next: props.next } }))
</script>

<template>
  <div class="box" role="note" :data-testid="`sealed-${kind}`">
    <template v-if="kind === 'mfa'">
      <p class="txt">{{ action ?? 'Managing the encryption keys' }} needs two-factor authentication on your account (owners only, for everyone's safety).</p>
      <NuxtLink :to="mfaRoute" class="link">SET UP 2FA →</NuxtLink>
    </template>
    <template v-else>
      <p class="txt">For safety, {{ (action ?? 'managing the encryption keys').toLowerCase() }} needs a recent sign-in (in the last 15 minutes). Sign in again and you'll come straight back here.</p>
      <NuxtLink :to="route" class="link">SIGN IN AGAIN →</NuxtLink>
    </template>
  </div>
</template>

<style scoped>
.box {
  display: grid;
  gap: 2px;
  padding: 10px 12px;
  border-left: 3px solid var(--color-status-archived);
  background: var(--color-surface-container);
}
.txt {
  margin: 0;
  font-size: 13px;
  color: var(--color-on-surface);
}
.link {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  justify-self: start;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .06em;
  color: var(--color-primary);
}
</style>
