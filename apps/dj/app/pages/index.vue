<script setup lang="ts">
import { Send, Layers, TrendingUp } from 'lucide-vue-next'

useHead({ title: 'Dashboard — KlubHub DJ' })

const upcomingGig = {
  venue: 'CYBER VOID',
  subtitle: 'MAINSTAGE — BERLIN',
  date: 'SAT 29 MAR 2026',
  fee: '€1,200',
  status: 'CONFIRMED',
  set: 'TECHNO RITUAL VOL. 4 · 47 TRK',
}

const socialQueue = [
  {
    id: 1,
    caption: 'Techno Ritual Vol. 4 — 47 tracks from Cyber Void Berlin',
    status: 'SCHEDULED',
    time: 'MAR 29 · 23:45',
    platform: 'INSTAGRAM',
    accentClass: 'accent-bar-ready',
    badgeClass: 'badge-scheduled',
    errorMsg: '',
  },
  {
    id: 2,
    caption: 'Closing set highlights from Tresor last weekend...',
    status: 'READY',
    time: 'APR 02 · 20:00',
    platform: 'INSTAGRAM',
    accentClass: 'accent-bar-draft',
    badgeClass: 'badge-draft',
    errorMsg: '',
  },
  {
    id: 3,
    caption: 'Studio session warmup — deep minimal techno',
    status: 'FAILED',
    time: 'MAR 18',
    platform: 'INSTAGRAM',
    accentClass: 'accent-bar-failed',
    badgeClass: 'badge-failed',
    errorMsg: 'SYNC ERROR — RETRY',
  },
]

const recentTracklists = [
  { id: 1, title: 'TECHNO RITUAL VOL. 4', tracks: 47, bpm: 142, status: 'ready',    accentClass: 'accent-bar-ready',    badgeClass: 'badge-ready',    badgeLabel: 'READY' },
  { id: 2, title: 'TRESOR CLOSING SET',   tracks: 31, bpm: 138, status: 'draft',    accentClass: 'accent-bar-draft',    badgeClass: 'badge-draft',    badgeLabel: 'DRAFT' },
  { id: 3, title: 'STUDIO WARMUP',        tracks: 18, bpm: 128, status: 'archived', accentClass: 'accent-bar-archived', badgeClass: 'badge-archived', badgeLabel: 'ARC' },
]

const finance = {
  mtdEarned: '€3,400',
  pending: '€1,200',
  ytd: '€14,800',
  nextPayout: 'PAYOUT APR 01',
}
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">

    <!-- Page header -->
    <div class="page-header">
      <div>
        <div class="page-title">DASHBOARD</div>
        <div class="page-sub">OVERVIEW — MAR 2026</div>
      </div>
    </div>

    <!-- Scrollable body -->
    <div class="page-body">

      <!-- ── Hero: Upcoming gig card ── -->
      <div class="glass hud-card glow-card" style="display:flex;overflow:hidden;" aria-label="Upcoming gig">
        <!-- Cyan accent bar -->
        <div style="width:3px;flex-shrink:0;background:linear-gradient(180deg,var(--color-primary),var(--color-primary-container));" aria-hidden="true" />

        <div style="flex:1;padding:16px 18px;min-width:0;">
          <!-- Badges -->
          <div style="display:flex;gap:8px;margin-bottom:10px;flex-wrap:wrap;">
            <span class="badge-hud badge-ready" style="border-color:rgba(150,248,255,.5);box-shadow:0 0 12px rgba(150,248,255,.1);">⚡ UPCOMING</span>
            <span class="badge-hud badge-ready">{{ upcomingGig.status }}</span>
          </div>

          <!-- Venue — large display typography -->
          <div
            style="font-family:var(--font-command);font-weight:700;color:var(--color-primary);letter-spacing:-.04em;line-height:.9;text-shadow:0 0 60px rgba(150,248,255,.25);text-transform:uppercase;margin-bottom:4px;"
            :style="{ fontSize: 'clamp(32px,5vw,64px)' }"
          >
            {{ upcomingGig.venue }}
          </div>
          <div style="font-family:var(--font-command);font-size:clamp(12px,2vw,18px);font-weight:300;color:var(--color-on-surface-variant);letter-spacing:.08em;text-transform:uppercase;margin-bottom:10px;">
            {{ upcomingGig.subtitle }}
          </div>

          <!-- Meta row -->
          <div style="display:flex;flex-wrap:wrap;gap:16px;">
            <div style="display:flex;gap:6px;align-items:center;">
              <span class="section-lbl">DATE</span>
              <span style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-on-surface);">{{ upcomingGig.date }}</span>
            </div>
            <div style="display:flex;gap:6px;align-items:center;">
              <span class="section-lbl">FEE</span>
              <span style="font-family:var(--font-command);font-size:13px;font-weight:700;color:var(--color-primary);">{{ upcomingGig.fee }}</span>
            </div>
            <div style="display:flex;gap:6px;align-items:center;" class="hidden md:flex">
              <span class="section-lbl">SET</span>
              <span style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-on-surface);">{{ upcomingGig.set }}</span>
            </div>
          </div>
        </div>

        <!-- Action buttons -->
        <div
          style="display:flex;flex-direction:column;gap:6px;padding:14px 16px;border-left:1px solid rgba(150,248,255,.08);justify-content:center;flex-shrink:0;"
          class="hero-actions"
        >
          <NuxtLink
            to="/tracklist"
            class="btn-hud btn-hud-ghost"
            style="display:inline-flex;align-items:center;justify-content:center;gap:6px;padding:0 16px;"
          >
            <Layers style="width:12px;height:12px;flex-shrink:0;" aria-hidden="true" />
            TRACKLIST
          </NuxtLink>
          <NuxtLink
            to="/social"
            class="btn-hud btn-hud-cta"
            style="display:inline-flex;align-items:center;justify-content:center;gap:6px;padding:0 16px;"
          >
            <Send style="width:12px;height:12px;flex-shrink:0;" aria-hidden="true" />
            SCHEDULE
          </NuxtLink>
        </div>
      </div>

      <!-- ── Widget grid: 3 columns on desktop ── -->
      <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:14px;" class="col-grid">

        <!-- Column 1: Social Queue -->
        <section aria-label="Social queue" style="display:flex;flex-direction:column;gap:7px;">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:2px;">
            <div class="section-lbl">SOCIAL QUEUE</div>
            <NuxtLink to="/social" style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary);opacity:.7;cursor:pointer;text-decoration:none;" @mouseenter="($event.target as HTMLElement).style.opacity='1'" @mouseleave="($event.target as HTMLElement).style.opacity='.7'">VIEW ALL →</NuxtLink>
          </div>

          <NuxtLink
            v-for="post in socialQueue"
            :key="post.id"
            to="/social"
            class="glass spost"
            :class="post.accentClass"
            style="text-decoration:none;cursor:pointer;"
          >
            <div style="display:flex;justify-content:space-between;align-items:center;">
              <span class="badge-hud" :class="post.badgeClass">
                <span v-if="post.status === 'SCHEDULED'" style="width:4px;height:4px;background:var(--color-on-primary);display:inline-block;" />
                {{ post.status }}
              </span>
              <span class="spost-meta">{{ post.platform }}</span>
            </div>
            <div class="spost-caption">{{ post.caption }}</div>
            <div class="spost-meta" :style="post.status === 'FAILED' ? 'color:var(--color-error)' : ''">
              <svg v-if="post.status !== 'FAILED'" width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
              <svg v-else width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/></svg>
              {{ post.status === 'FAILED' ? post.errorMsg : post.time }}
            </div>
          </NuxtLink>
        </section>

        <!-- Column 2: Tracklists -->
        <section aria-label="Recent tracklists" style="display:flex;flex-direction:column;gap:7px;">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:2px;">
            <div class="section-lbl">TRACKLISTS</div>
            <NuxtLink to="/tracklist" style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary);opacity:.7;cursor:pointer;text-decoration:none;" @mouseenter="($event.target as HTMLElement).style.opacity='1'" @mouseleave="($event.target as HTMLElement).style.opacity='.7'">VIEW ALL →</NuxtLink>
          </div>

          <NuxtLink
            v-for="tl in recentTracklists"
            :key="tl.id"
            to="/tracklist"
            class="glass"
            :class="tl.accentClass"
            style="display:flex;align-items:center;justify-content:space-between;padding:10px 13px;cursor:pointer;transition:all .15s;text-decoration:none;"
          >
            <div>
              <div style="font-family:var(--font-command);font-size:11px;font-weight:600;color:var(--color-on-surface);text-transform:uppercase;letter-spacing:-.02em;">{{ tl.title }}</div>
              <div style="display:flex;gap:6px;margin-top:3px;">
                <span class="data-frag">{{ tl.tracks }} TRK</span>
                <span class="data-frag">{{ tl.bpm }} BPM</span>
              </div>
            </div>
            <span class="badge-hud" :class="tl.badgeClass">{{ tl.badgeLabel }}</span>
          </NuxtLink>

          <NuxtLink
            to="/tracklist"
            class="btn-hud btn-hud-ghost"
            style="display:flex;align-items:center;justify-content:center;gap:6px;width:100%;text-decoration:none;"
          >
            + NEW TRACKLIST
          </NuxtLink>
        </section>

        <!-- Column 3: Finance -->
        <section aria-label="Financial overview" style="display:flex;flex-direction:column;gap:7px;">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:2px;">
            <div class="section-lbl">FINANCE</div>
            <NuxtLink to="/finance" style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary);opacity:.7;cursor:pointer;text-decoration:none;" @mouseenter="($event.target as HTMLElement).style.opacity='1'" @mouseleave="($event.target as HTMLElement).style.opacity='.7'">VIEW ALL →</NuxtLink>
          </div>

          <div class="glass accent-bar-ready" style="padding:12px 14px;">
            <div class="section-lbl" style="margin-bottom:4px;">EARNED — MAR</div>
            <div style="font-family:var(--font-command);font-size:22px;font-weight:700;color:var(--color-primary);letter-spacing:-.02em;text-shadow:0 0 30px rgba(150,248,255,.2);">{{ finance.mtdEarned }}</div>
          </div>

          <div class="glass accent-bar-draft" style="padding:12px 14px;">
            <div class="section-lbl" style="margin-bottom:4px;">PENDING</div>
            <div style="font-family:var(--font-command);font-size:22px;font-weight:700;color:var(--color-secondary);letter-spacing:-.02em;">{{ finance.pending }}</div>
            <div class="spost-meta" style="margin-top:3px;">
              <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
              {{ finance.nextPayout }}
            </div>
          </div>

          <div class="glass" style="padding:12px 14px;">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px;">
              <div class="section-lbl">YTD TOTAL</div>
              <TrendingUp style="width:12px;height:12px;color:var(--color-primary);opacity:.5;" />
            </div>
            <div style="font-family:var(--font-command);font-size:18px;font-weight:700;color:var(--color-on-surface);letter-spacing:-.02em;">{{ finance.ytd }}</div>
          </div>
        </section>

      </div>
    </div>
  </div>
</template>

<style scoped>
/* Mobile: stack columns */
@media (max-width: 768px) {
  .col-grid { grid-template-columns: 1fr !important; }
  .hero-actions {
    flex-direction: row !important;
    border-left: 0 !important;
    border-top: 1px solid rgba(150,248,255,.08) !important;
  }
}
</style>
