<script setup lang="ts">
/** Centered card for the pages shown without the app shell (login, setup). */
defineProps<{ title: string, subtitle?: string }>()
</script>

<template>
  <main class="hud-bg min-h-dvh flex items-center justify-center p-4">
    <section class="glass hud-card w-full max-w-md" style="padding:28px;" :aria-labelledby="'auth-title'">
      <div class="bracket-box" style="padding:10px 12px;margin-bottom:22px;">
        <div style="font-family:var(--font-command);font-size:14px;font-weight:700;text-transform:uppercase;letter-spacing:-.02em;">
          KlubHub Promoter
        </div>
      </div>
      <h1 id="auth-title" class="page-title" style="margin:0 0 4px;">{{ title }}</h1>
      <p v-if="subtitle" class="page-sub" style="margin:0 0 20px;">{{ subtitle }}</p>
      <slot />
    </section>
  </main>
</template>
