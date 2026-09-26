<script setup lang="ts">
/**
 * PhasePlaceholder — honest empty state for sections that land in a later
 * phase of the promoter plan (.claude/plans/promoter-app.plan.md §5).
 */
defineProps<{
  title: string
  phase: string
  summary: string
  features: string[]
}>()
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <div class="page-title">{{ title }}</div>
        <div class="page-sub">{{ phase }}</div>
      </div>
    </div>
    <div class="page-body">
      <section class="glass hud-card" style="padding:24px;max-width:720px;">
        <div class="section-lbl" style="margin-bottom:10px;">COMING IN {{ phase }}</div>
        <p style="font-family:var(--font-data);font-size:14px;color:var(--color-on-surface);margin:0 0 14px;">
          {{ summary }}
        </p>
        <ul style="margin:0;padding-left:18px;display:grid;gap:6px;">
          <li
            v-for="feature in features"
            :key="feature"
            style="font-family:var(--font-data);font-size:13px;color:var(--color-on-surface-variant);"
          >
            {{ feature }}
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>
