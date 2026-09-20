<script setup lang="ts">
import { Send, Layers, TrendingUp, Zap } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useGigStore } from '~/stores/gig'
import { useSocialStore } from '~/stores/social'
import { useTracklistStore } from '~/stores/tracklist'

useHead({ title: 'Dashboard — KlubHub DJ' })

const gigStore = useGigStore()
const socialStore = useSocialStore()
const tracklistStore = useTracklistStore()

const { gigs, upcomingGigs, ytdEarned, avgFee } = storeToRefs(gigStore)
const { posts: socialPosts } = storeToRefs(socialStore)
const { pastTracklists } = storeToRefs(tracklistStore)

// Load data on mount
onMounted(async () => {
  await Promise.all([
    gigStore.fetchGigs(),
    socialStore.loadPosts(),
    tracklistStore.loadPastTracklists(),
  ])
})

// Helpers for status badge classes
function statusBadgeClass(status: string): string {
  switch (status) {
    case 'confirmed': return 'badge-ready'
    case 'advanced': return 'badge-ready'
    case 'played': return 'badge-ready'
    case 'inquiry': return 'badge-draft'
    case 'cancelled': return 'badge-archived'
    default: return 'badge-draft'
  }
}

function statusAccentClass(status: string): string {
  switch (status) {
    case 'confirmed': return 'accent-bar-ready'
    case 'advanced': return 'accent-bar-ready'
    case 'played': return 'accent-bar-ready'
    case 'inquiry': return 'accent-bar-draft'
    case 'cancelled': return 'accent-bar-archived'
    default: return 'accent-bar-draft'
  }
}

function formatDateTime(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleDateString('en-GB', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit', hour12: false }).toUpperCase().replace(',', ' ·')
}

function formatCurrency(amount: number, currency = 'EUR'): string {
  return new Intl.NumberFormat('de-DE', { style: 'currency', currency }).format(amount)
}

// Computed: upcoming gig (first confirmed/advanced/played gig in the future)
const upcomingGig = computed(() => {
  const now = new Date()
  return upcomingGigs.value
    .filter(g => new Date(g.date) >= now)
    .sort((a, b) => new Date(a.date).getTime() - new Date(b.date).getTime())[0] || null
})

// Computed: social queue (top 3 posts)
const socialQueue = computed(() => {
  return socialPosts.value.slice(0, 3).map(post => ({
    ...post,
    badgeClass: statusBadgeClass(post.status),
    accentClass: statusAccentClass(post.status),
    time: formatDateTime(post.scheduledAtUtc),
    platform: socialStore.account?.platform?.toUpperCase() || 'INSTAGRAM',
  }))
})

// Computed: recent tracklists (top 3 by createdAt desc)
const recentTracklists = computed(() => {
  return pastTracklists.value.slice(0, 3).map(tl => ({
    ...tl,
    badgeClass: statusBadgeClass(tl.status || 'ready'),
    accentClass: statusAccentClass(tl.status || 'ready'),
    badgeLabel: (tl.status || 'READY').toUpperCase().slice(0, 3),
  }))
})

// Computed: finance data from played gigs
const finance = computed(() => {
  const played = gigs.value.filter(g => g.status === 'played')
  const currentYear = new Date().getFullYear()
  const thisMonth = played.filter(g => {
    const d = new Date(g.date)
    return d.getFullYear() === currentYear && d.getMonth() === new Date().getMonth()
  })
  const mtd = thisMonth.reduce((sum, g) => sum + (g.fee_amount || 0), 0)
  const pending = upcomingGigs.value.reduce((sum, g) => sum + (g.fee_amount || 0), 0)
  const ytd = ytdEarned.value
  return {
    mtd,
    pending,
    ytd,
    avgFee: avgFee.value,
  }
})
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">
    <!-- Page header -->
    <div class="page-header">
      <div>
        <div class="page-title">DASHBOARD</div>
        <div class="page-sub">OVERVIEW</div>
      </div>
    </div>

    <!-- Scrollable body -->
    <div class="page-body">
      <!-- ── Hero: Upcoming gig card ── -->
      <div v-if="upcomingGig" class="glass hud-card glow-card accent-bar-ready" style="display:flex;overflow:hidden;" aria-label="Upcoming gig">
        <div style="flex:1;padding:16px 18px;min-width:0;">
          <!-- Badges -->
          <div style="display:flex;gap:8px;margin-bottom:10px;flex-wrap:wrap;">
            <span class="badge-hud badge-ready" style="border-color:rgba(150,248,255,.5);box-shadow:0 0 12px rgba(150,248,255,.1);">
              <Zap style="width:8px;height:8px;" aria-hidden="true" />
              UPCOMING
            </span>
            <span class="badge-hud badge-ready">{{ upcomingGig.status.toUpperCase() }}</span>
          </div>

          <!-- Venue — large display typography -->
          <div
            style="font-family:var(--font-command);font-weight:700;color:var(--color-primary);letter-spacing:-.04em;line-height:.9;text-shadow:0 0 60px rgba(150,248,255,.25);text-transform:uppercase;margin-bottom:4px;"
            :style="{ fontSize: 'clamp(32px,5vw,64px)' }"
          >
            {{ upcomingGig.venue }}
          </div>
          <div style="font-family:var(--font-command);font-size:clamp(12px,2vw,18px);font-weight:300;color:var(--color-on-surface-variant);letter-spacing:.08em;text-transform:uppercase;margin-bottom:10px;">
            {{ upcomingGig.city }}{{ upcomingGig.country ? ', ' + upcomingGig.country : '' }}
          </div>

          <!-- Meta row -->
          <div style="display:flex;flex-wrap:wrap;gap:16px;">
            <div style="display:flex;gap:6px;align-items:center;">
              <span class="section-lbl">DATE</span>
              <span style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-on-surface);">{{ formatDateTime(upcomingGig.date) }}</span>
            </div>
            <div style="display:flex;gap:6px;align-items:center;">
              <span class="section-lbl">FEE</span>
              <span style="font-family:var(--font-command);font-size:13px;font-weight:700;color:var(--color-primary);">{{ formatCurrency(upcomingGig.fee_amount, upcomingGig.fee_currency) }}</span>
            </div>
            <div style="display:flex;gap:6px;align-items:center;" class="hidden md:flex">
              <span class="section-lbl">SET</span>
              <span style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-on-surface);">{{ upcomingGig.set_length_minutes }} MIN</span>
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

      <!-- Empty state: no upcoming gigs -->
      <div v-else class="glass hud-card accent-bar-draft" style="padding:24px;text-align:center;">
        <Zap style="width:48px;height:48px;color:var(--color-tertiary);margin:0 auto 12px;" aria-hidden="true" />
        <div style="font-family:var(--font-command);font-size:16px;font-weight:600;color:var(--color-on-surface);margin-bottom:4px;">NO UPCOMING GIGS</div>
        <div style="font-family:var(--font-data);font-size:12px;color:var(--color-on-surface-variant);">Create your first gig to see it here.</div>
        <NuxtLink to="/gigs" class="btn-hud btn-hud-cta" style="margin-top:16px;display:inline-block;">+ NEW GIG</NuxtLink>
      </div>

      <!-- ── Widget grid: 3 columns on desktop ── -->
      <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:14px;" class="col-grid">
        <!-- Column 1: Social Queue -->
        <section aria-label="Social queue" style="display:flex;flex-direction:column;gap:7px;">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:2px;">
            <div class="section-lbl">SOCIAL QUEUE</div>
            <NuxtLink to="/social" style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary);opacity:.7;cursor:pointer;text-decoration:none;" @mouseenter="($event.target as HTMLElement).style.opacity='1'" @mouseleave="($event.target as HTMLElement).style.opacity='.7'">VIEW ALL →</NuxtLink>
          </div>

          <template v-if="socialQueue.length > 0">
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
                  <span v-if="post.status === 'scheduled'" style="width:4px;height:4px;background:var(--color-on-primary);display:inline-block;" />
                  {{ post.status.toUpperCase() }}
                </span>
                <span class="spost-meta">{{ post.platform }}</span>
              </div>
              <div class="spost-caption">{{ post.caption }}</div>
              <div class="spost-meta" :style="post.status === 'failed' ? 'color:var(--color-error)' : ''">
                <svg v-if="post.status !== 'failed'" width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
                <svg v-else width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/></svg>
                {{ post.status === 'failed' ? post.lastError : post.time }}
              </div>
            </NuxtLink>
          </template>

          <div v-else class="glass accent-bar-draft" style="padding:16px;text-align:center;">
            <Send style="width:24px;height:24px;color:var(--color-tertiary);margin:0 auto 8px;" aria-hidden="true" />
            <div style="font-family:var(--font-command);font-size:12px;color:var(--color-on-surface-variant);">QUEUE IS EMPTY</div>
            <div style="font-family:var(--font-data);font-size:10px;color:var(--color-tertiary);margin-top:4px;">Schedule a post to get started.</div>
          </div>
        </section>

        <!-- Column 2: Tracklists -->
                <section aria-label="Recent tracklists" style="display:flex;flex-direction:column;gap:7px;">
                  <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:2px;">
                    <div class="section-lbl">TRACKLISTS</div>
                    <NuxtLink to="/tracklist" style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary);opacity:.7;cursor:pointer;text-decoration:none;" @mouseenter="($event.target as HTMLElement).style.opacity='1'" @mouseleave="($event.target as HTMLElement).style.opacity='.7'">VIEW ALL →</NuxtLink>
                  </div>

                  <template v-if="recentTracklists.length > 0">
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
                  </template>

                  <div v-else class="glass accent-bar-draft" style="padding:16px;text-align:center;">
                    <Layers style="width:24px;height:24px;color:var(--color-tertiary);margin:0 auto 8px;" aria-hidden="true" />
                    <div style="font-family:var(--font-command);font-size:12px;color:var(--color-on-surface-variant);">NO TRACKLISTS YET</div>
                    <div style="font-family:var(--font-data);font-size:10px;color:var(--color-tertiary);margin-top:4px;">Upload a tracklist to get started.</div>
                  </div>

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
            <div class="section-lbl" style="margin-bottom:4px;">EARNED THIS MONTH</div>
            <div style="font-family:var(--font-command);font-size:22px;font-weight:700;color:var(--color-primary);letter-spacing:-.02em;text-shadow:0 0 30px rgba(150,248,255,.2);">{{ formatCurrency(finance.mtd) }}</div>
            <div v-if="finance.mtd === 0" class="spost-meta" style="margin-top:4px;font-size:10px;">No played gigs this month</div>
          </div>

          <div class="glass accent-bar-draft" style="padding:12px 14px;">
            <div class="section-lbl" style="margin-bottom:4px;">PENDING</div>
            <div style="font-family:var(--font-command);font-size:22px;font-weight:700;color:var(--color-secondary);letter-spacing:-.02em;">{{ formatCurrency(finance.pending) }}</div>
            <div class="spost-meta" style="margin-top:3px;">
              <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
              From upcoming confirmed gigs
            </div>
          </div>

          <div class="glass" style="padding:12px 14px;">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px;">
              <div class="section-lbl">YTD TOTAL</div>
              <TrendingUp style="width:12px;height:12px;color:var(--color-tertiary);opacity:.5;" />
            </div>
            <div style="font-family:var(--font-command);font-size:18px;font-weight:700;color:var(--color-on-surface);letter-spacing:-.02em;">{{ formatCurrency(finance.ytd) }}</div>
            <div v-if="finance.ytd === 0" class="spost-meta" style="margin-top:4px;font-size:10px;">No played gigs this year</div>
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