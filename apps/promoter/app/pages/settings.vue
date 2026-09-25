<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useOrgStore } from '~/stores/org'
import { useSessionStore } from '~/stores/session'
import type { ApiError } from '~/types/event'
import type { OrgProfile } from '~/types/org'

useHead({ title: 'Settings' })

const store = useOrgStore()
const { org, error } = storeToRefs(store)
const { canManageOrg } = storeToRefs(useSessionStore())
await callOnce('promoter-org', () => store.fetchOrg())

const busy = ref(false)
const serverError = ref<ApiError | null>(null)
const notice = ref<string | null>(null)
const formKey = ref(0)

async function save(p: OrgProfile) {
  busy.value = true
  serverError.value = null
  notice.value = null
  try {
    await store.updateProfile(p)
    notice.value = 'PROFILE SAVED · REBUILD EXPORT PACKS TO USE IT'
    formKey.value++
  } catch (e) {
    const err = e as ApiError
    serverError.value = err
    if (!err.field) notice.value = err.error === 'no_role_grant' ? 'Only owners and admins can change the profile.' : 'Couldn\'t save the profile. Try again.'
  } finally {
    busy.value = false
  }
}

const coming = ['Team members and roles', 'Door devices and PINs', 'Data retention and encryption status']
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">SETTINGS</h1>
        <div class="page-sub">{{ org ? `${org.name} · ${org.timezone} · ${org.currency}` : 'ORGANISATION' }}</div>
      </div>
    </div>
    <div class="page-body space-y-4">
      <p v-if="error" role="alert" class="glass accent-bar-failed" style="padding:12px 16px;">
        COULDN'T LOAD THE ORGANISATION. <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="store.fetchOrg()">RETRY</button>
      </p>
      <section v-if="org" aria-labelledby="profile-h" class="space-y-3">
        <h2 id="profile-h" class="section-lbl" style="margin:0;">COLLECTIVE PROFILE</h2>
        <p v-if="notice" role="status" class="data-frag" style="font-size:9px;">{{ notice }}</p>
        <p v-if="!canManageOrg" class="glass" style="padding:10px 14px;font-size:13px;margin:0;">READ ONLY · Owners and admins can change the profile.</p>
        <SettingsProfileForm :key="formKey" :org="org" :busy="busy" :server-error="serverError" :read-only="!canManageOrg" @submit="save" />
      </section>
      <section aria-labelledby="coming-h" class="glass" style="padding:16px;">
        <h2 id="coming-h" class="section-lbl" style="margin:0 0 6px;">COMING NEXT</h2>
        <ul style="margin:0;padding-left:18px;font-size:13px;color:var(--color-on-surface-variant);">
          <li v-for="c in coming" :key="c">{{ c }}</li>
        </ul>
        <NuxtLink to="/account/security" class="btn-hud btn-hud-ghost" style="margin-top:10px;min-height:44px;">YOUR ACCOUNT SECURITY →</NuxtLink>
      </section>
    </div>
  </div>
</template>
