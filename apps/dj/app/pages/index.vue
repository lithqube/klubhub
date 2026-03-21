<script setup lang="ts">
import { Send, Layers, TrendingUp, AlertTriangle, CheckCircle2, Clock } from 'lucide-vue-next'

useHead({ title: 'Dashboard — KlubHub DJ' })

// ── Static mock data (real data will come from API in future phases) ──

const upcomingGig = {
  tag: 'UPCOMING PRIORITY',
  venue: 'CYBER VOID',
  subtitle: 'MAINSTAGE',
  date: 'SAT 29 MAR 2026',
  location: 'BERLIN, DE',
  fee: '€1,200',
  status: 'CONFIRMED',
  tracklist: 'Techno Ritual Vol. 4',
  tracks: 47,
}

const socialQueue = [
  { id: 1, caption: 'Techno Ritual Vol. 4 — 47 tracks • avg 142 BPM', status: 'SCHEDULED', time: 'MAR 29, 23:45', platform: 'INSTAGRAM' },
  { id: 2, caption: 'Closing set highlights from Tresor last weekend...', status: 'READY',     time: 'APR 02, 20:00', platform: 'INSTAGRAM' },
  { id: 3, caption: 'Studio session warmup — deep minimal techno vibes', status: 'FAILED',    time: 'MAR 18, 18:30', platform: 'INSTAGRAM' },
]

const recentTracklists = [
  { id: 1, title: 'Techno Ritual Vol. 4',   tracks: 47, bpm: 142, status: 'ready'    },
  { id: 2, title: 'Tresor Closing Set',      tracks: 31, bpm: 138, status: 'draft'    },
  { id: 3, title: 'Studio Warmup — Minimal', tracks: 18, bpm: 128, status: 'archived' },
]

const finance = {
  mtdEarned: '€3,400',
  pending: '€1,200',
  ytd: '€14,800',
  nextPayout: 'APR 01, 2026',
}

type SocialStatus = 'SCHEDULED' | 'READY' | 'FAILED'

function socialStatusClass(status: SocialStatus): string {
  const map: Record<SocialStatus, string> = {
    SCHEDULED: 'text-primary bg-primary/10',
    READY:     'text-secondary bg-secondary/10',
    FAILED:    'text-error bg-error/10',
  }
  return map[status] ?? 'text-tertiary bg-surface-container'
}

type TracklistStatus = 'ready' | 'draft' | 'archived'

function accentBar(status: TracklistStatus): string {
  const map: Record<TracklistStatus, string> = {
    ready:    'accent-bar-ready',
    draft:    'accent-bar-draft',
    archived: 'accent-bar-archived',
  }
  return map[status] ?? ''
}
</script>

<template>
  <div class="flex-1 flex flex-col overflow-hidden">

    <!-- Page header -->
    <header class="px-8 py-5 border-b border-outline-variant/20 flex items-center justify-between flex-shrink-0">
      <div>
        <h1 class="font-command font-bold text-on-surface text-xl uppercase tracking-wide">
          DASHBOARD
        </h1>
        <p class="font-terminal tracking-terminal text-tertiary text-xs uppercase mt-0.5">
          OVERVIEW — MAR 2026
        </p>
      </div>
    </header>

    <!-- Scrollable body -->
    <main class="flex-1 overflow-y-auto px-8 py-6 space-y-6">

      <!-- ── Hero: Upcoming gig card ── -->
      <section aria-label="Upcoming gig">
        <div class="glass-panel border border-outline-variant/30 flex overflow-hidden">

          <!-- Left cyan accent bar -->
          <div class="w-1 flex-shrink-0 gradient-cta" aria-hidden="true" />

          <div class="flex-1 px-6 py-5">

            <!-- Tag row -->
            <div class="flex items-center gap-3 mb-3">
              <span class="font-terminal tracking-terminal text-xs text-primary uppercase px-2 py-0.5 border border-primary/40">
                ⚡ {{ upcomingGig.tag }}
              </span>
              <span class="font-terminal tracking-terminal text-xs text-on-surface uppercase px-2 py-0.5 bg-primary/10 border border-primary/20">
                {{ upcomingGig.status }}
              </span>
            </div>

            <!-- Venue headline — mixed weight -->
            <div class="mb-4">
              <p class="font-command text-4xl font-bold text-primary leading-none uppercase tracking-tight">
                {{ upcomingGig.venue }}
              </p>
              <p class="font-command text-2xl font-light text-on-surface-variant uppercase tracking-widest mt-0.5">
                {{ upcomingGig.subtitle }}
              </p>
            </div>

            <!-- Meta row -->
            <div class="flex flex-wrap items-center gap-x-6 gap-y-2">
              <div class="flex items-center gap-2">
                <span class="font-terminal tracking-terminal text-xs text-tertiary uppercase">DATE</span>
                <span class="font-terminal tracking-terminal text-xs text-on-surface uppercase">{{ upcomingGig.date }}</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="font-terminal tracking-terminal text-xs text-tertiary uppercase">LOCATION</span>
                <span class="font-terminal tracking-terminal text-xs text-on-surface uppercase">{{ upcomingGig.location }}</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="font-terminal tracking-terminal text-xs text-tertiary uppercase">FEE</span>
                <span class="font-terminal tracking-terminal text-xs text-primary font-bold uppercase">{{ upcomingGig.fee }}</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="font-terminal tracking-terminal text-xs text-tertiary uppercase">TRACKLIST</span>
                <span class="font-terminal tracking-terminal text-xs text-on-surface uppercase">
                  {{ upcomingGig.tracklist }} ({{ upcomingGig.tracks }} tracks)
                </span>
              </div>
            </div>

          </div>

          <!-- Right actions -->
          <div class="flex flex-col justify-center gap-3 px-6 border-l border-outline-variant/20 flex-shrink-0">
            <NuxtLink
              to="/tracklist"
              class="flex items-center gap-2 px-4 py-2 ghost-border text-on-surface-variant hover:text-primary hover:border-primary/60 transition-colors"
            >
              <Layers class="w-3.5 h-3.5" />
              <span class="font-terminal tracking-terminal text-xs uppercase">TRACKLIST</span>
            </NuxtLink>
            <NuxtLink
              to="/social"
              class="flex items-center gap-2 px-4 py-2 gradient-cta text-on-primary font-terminal tracking-terminal text-xs uppercase"
            >
              <Send class="w-3.5 h-3.5" />
              <span>SCHEDULE POST</span>
            </NuxtLink>
          </div>

        </div>
      </section>

      <!-- ── Three-column widgets ── -->
      <div class="grid grid-cols-3 gap-6">

        <!-- Column 1: Social Queue -->
        <section aria-label="Social queue" class="flex flex-col gap-3">
          <div class="flex items-center justify-between">
            <h2 class="font-terminal tracking-terminal text-xs text-tertiary uppercase">
              SOCIAL QUEUE
            </h2>
            <NuxtLink
              to="/social"
              class="font-terminal tracking-terminal text-xs text-primary/70 hover:text-primary uppercase transition-colors"
            >
              VIEW ALL →
            </NuxtLink>
          </div>

          <div
            v-for="post in socialQueue"
            :key="post.id"
            class="glass-panel border border-outline-variant/20 p-4"
          >
            <!-- Status + platform -->
            <div class="flex items-center justify-between mb-2">
              <span
                class="font-terminal tracking-terminal text-[10px] uppercase px-1.5 py-0.5"
                :class="socialStatusClass(post.status as any)"
              >
                {{ post.status }}
              </span>
              <span class="font-terminal tracking-terminal text-[10px] text-tertiary uppercase">
                {{ post.platform }}
              </span>
            </div>

            <!-- Caption excerpt -->
            <p class="font-data text-xs text-on-surface-variant leading-relaxed line-clamp-2 mb-2">
              {{ post.caption }}
            </p>

            <!-- Time -->
            <div class="flex items-center gap-1.5">
              <Clock class="w-3 h-3 text-tertiary flex-shrink-0" />
              <span class="font-terminal tracking-terminal text-[10px] text-tertiary uppercase">
                {{ post.time }}
              </span>
            </div>

            <!-- Failed indicator -->
            <div
              v-if="post.status === 'FAILED'"
              class="flex items-center gap-1.5 mt-2 pt-2 border-t border-error/20"
            >
              <AlertTriangle class="w-3 h-3 text-error flex-shrink-0" />
              <span class="font-terminal tracking-terminal text-[10px] text-error uppercase">
                SYNC ERROR — RETRY REQUIRED
              </span>
            </div>
          </div>
        </section>

        <!-- Column 2: Recent Tracklists -->
        <section aria-label="Recent tracklists" class="flex flex-col gap-3">
          <div class="flex items-center justify-between">
            <h2 class="font-terminal tracking-terminal text-xs text-tertiary uppercase">
              RECENT TRACKLISTS
            </h2>
            <NuxtLink
              to="/tracklist"
              class="font-terminal tracking-terminal text-xs text-primary/70 hover:text-primary uppercase transition-colors"
            >
              VIEW ALL →
            </NuxtLink>
          </div>

          <div
            v-for="tl in recentTracklists"
            :key="tl.id"
            class="glass-panel border border-outline-variant/20 flex overflow-hidden"
            :class="accentBar(tl.status as any)"
          >
            <div class="flex-1 px-4 py-3">
              <p class="font-command text-sm font-semibold text-on-surface uppercase mb-1">
                {{ tl.title }}
              </p>
              <div class="flex items-center gap-4">
                <span class="font-terminal tracking-terminal text-[10px] text-tertiary uppercase">
                  {{ tl.tracks }} TRACKS
                </span>
                <span class="font-terminal tracking-terminal text-[10px] text-tertiary uppercase">
                  AVG {{ tl.bpm }} BPM
                </span>
              </div>
            </div>
            <div class="flex items-center px-4 border-l border-outline-variant/20">
              <CheckCircle2
                v-if="tl.status === 'ready'"
                class="w-4 h-4 text-primary"
              />
              <span
                v-else
                class="font-terminal tracking-terminal text-[10px] uppercase"
                :class="tl.status === 'draft' ? 'text-secondary' : 'text-status-archived'"
              >
                {{ tl.status.toUpperCase() }}
              </span>
            </div>
          </div>

          <!-- Generate CTA -->
          <NuxtLink
            to="/tracklist"
            class="flex items-center justify-center gap-2 px-4 py-3 ghost-border text-tertiary hover:text-primary hover:border-primary/40 transition-colors"
          >
            <Layers class="w-3.5 h-3.5" />
            <span class="font-terminal tracking-terminal text-xs uppercase">+ NEW TRACKLIST</span>
          </NuxtLink>
        </section>

        <!-- Column 3: Financial Overview -->
        <section aria-label="Financial overview" class="flex flex-col gap-3">
          <div class="flex items-center justify-between">
            <h2 class="font-terminal tracking-terminal text-xs text-tertiary uppercase">
              FINANCIAL OVERVIEW
            </h2>
            <NuxtLink
              to="/finance"
              class="font-terminal tracking-terminal text-xs text-primary/70 hover:text-primary uppercase transition-colors"
            >
              VIEW ALL →
            </NuxtLink>
          </div>

          <!-- MTD earned -->
          <div class="glass-panel border border-outline-variant/20 p-4 accent-bar-confirmed">
            <p class="font-terminal tracking-terminal text-[10px] text-tertiary uppercase mb-1">
              EARNED THIS MONTH
            </p>
            <p class="font-command text-2xl font-bold text-primary">
              {{ finance.mtdEarned }}
            </p>
          </div>

          <!-- Pending -->
          <div class="glass-panel border border-outline-variant/20 p-4 accent-bar-draft">
            <p class="font-terminal tracking-terminal text-[10px] text-tertiary uppercase mb-1">
              PENDING PAYMENT
            </p>
            <p class="font-command text-2xl font-bold text-secondary">
              {{ finance.pending }}
            </p>
            <div class="flex items-center gap-1.5 mt-1.5">
              <Clock class="w-3 h-3 text-tertiary flex-shrink-0" />
              <span class="font-terminal tracking-terminal text-[10px] text-tertiary uppercase">
                NEXT PAYOUT {{ finance.nextPayout }}
              </span>
            </div>
          </div>

          <!-- YTD -->
          <div class="glass-panel border border-outline-variant/20 p-4">
            <div class="flex items-center justify-between mb-1">
              <p class="font-terminal tracking-terminal text-[10px] text-tertiary uppercase">
                YTD TOTAL
              </p>
              <TrendingUp class="w-3.5 h-3.5 text-primary/60" />
            </div>
            <p class="font-command text-xl font-bold text-on-surface">
              {{ finance.ytd }}
            </p>
          </div>
        </section>

      </div>

    </main>
  </div>
</template>
