<script setup lang="ts">
import type { Component } from 'vue'

/** KhEmptyState — one icon, one line of title, one hint, at most one action. */
defineProps<{
  title: string
  hint?: string
  icon?: Component
  actionLabel?: string
  actionTo?: string
}>()
defineEmits<{ action: [] }>()
</script>

<template>
  <div class="glass" style="padding:32px 20px;text-align:center;display:flex;flex-direction:column;align-items:center;gap:10px;">
    <component :is="icon" v-if="icon" style="width:22px;height:22px;color:var(--color-primary);" aria-hidden="true" />
    <div class="section-lbl" style="font-size:10px;">{{ title }}</div>
    <p v-if="hint" style="margin:0;font-family:var(--font-data);font-size:13px;color:var(--color-on-surface-variant);max-width:42ch;">
      {{ hint }}
    </p>
    <NuxtLink v-if="actionLabel && actionTo" :to="actionTo" class="btn-hud btn-hud-cta" style="min-height:44px;margin-top:6px;">
      {{ actionLabel }}
    </NuxtLink>
    <button v-else-if="actionLabel" type="button" class="btn-hud btn-hud-cta" style="min-height:44px;margin-top:6px;" @click="$emit('action')">
      {{ actionLabel }}
    </button>
  </div>
</template>
