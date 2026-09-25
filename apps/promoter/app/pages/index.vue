<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useOrgStore } from '~/stores/org'
import { useSessionStore } from '~/stores/session'

useHead({ title: 'Dashboard' })

const orgStore = useOrgStore()
const { org, loading, error } = storeToRefs(orgStore)
await callOnce('promoter-org', () => orgStore.fetchOrg())

const { needsMfa } = storeToRefs(useSessionStore())

const roadmap = [
  { phase: 'P1', title: 'Events & lineup', detail: 'Events, venues, stages, timetable, export pack for your own site.' },
  { phase: 'P2', title: 'Guest list & door', detail: 'Lists with per-DJ quotas, offline door app, reports.' },
  { phase: 'P3', title: 'Audience & promotion', detail: 'Consent-first audience, campaigns, social assets.' },
]
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <div class="page-title">DASHBOARD</div>
        <div class="page-sub">
          <template v-if="loading">LOADING ORGANISATION…</template>
          <template v-else-if="org">{{ org.name }} · {{ org.timezone }} · {{ org.currency }}</template>
          <template v-else>NO ORGANISATION</template>
        </div>
      </div>
    </div>

    <div class="page-body space-y-4">
      <NuxtLink
        v-if="needsMfa"
        to="/account/security"
        class="glass accent-bar-pending"
        style="display:block;padding:12px 16px;font-family:var(--font-data);font-size:13px;color:var(--color-on-surface);text-decoration:none;border-left:3px solid var(--color-secondary);"
      >
        <strong>Add an authenticator app.</strong> Your role needs a second factor to manage members, security and money. Set it up now →
      </NuxtLink>
      <p v-if="error" role="alert" class="glass" style="padding:12px 16px;color:var(--color-error);font-family:var(--font-data);font-size:13px;">
        {{ error }}
      </p>

      <section aria-labelledby="roadmap-title">
        <div id="roadmap-title" class="section-lbl" style="margin-bottom:10px;">BUILD PROGRESS</div>
        <div style="display:grid;gap:12px;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));">
          <article v-for="item in roadmap" :key="item.phase" class="glass hud-card" style="padding:18px;">
            <div class="section-lbl">{{ item.phase }}</div>
            <h2 style="font-family:var(--font-command);font-size:15px;font-weight:700;text-transform:uppercase;margin:6px 0;">
              {{ item.title }}
            </h2>
            <p style="font-family:var(--font-data);font-size:13px;color:var(--color-on-surface-variant);margin:0;">
              {{ item.detail }}
            </p>
          </article>
        </div>
      </section>
    </div>
  </div>
</template>
