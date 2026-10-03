<script setup lang="ts">
import { Send, Layers, TrendingUp, Zap, CalendarPlus, Plus } from 'lucide-vue-next'
import { computed } from 'vue'
import { useDashboard } from '../composables/useDashboard'

useHead({ title: 'Dashboard — KlubHub DJ' })

const { gigs, posts: socialPosts, pastTracklists, loading, gigError, socialError, tracklistError, demoData, financeUnavailable, pendingUnavailable, finance } = useDashboard()
// Gig dates are calendar days, not scheduled UTC instants. Keep today's gig
// visible all day and do not shift a date-only value across timezones.
const upcomingGigs = computed(() => {
  const now = new Date()
  const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
  return gigs.value.filter(g => !g.deleted_at && ['confirmed', 'advanced', 'played'].includes(g.status) && g.date.slice(0, 10) >= today)
    .sort((a, b) => a.date.localeCompare(b.date))
})

// Social statuses are distinct from gig booking statuses.
function statusBadgeClass(status: string): string {
  switch (status) {
    case 'scheduled':
    case 'publishing': return 'badge-ready'
    case 'published': return 'badge-archived'
    case 'failed':
    case 'permanently_failed': return 'badge-failed'
    default: return 'badge-draft'
  }
}

function statusAccentClass(status: string): string {
  switch (status) {
    case 'scheduled':
    case 'publishing': return 'accent-bar-ready'
    case 'published': return 'accent-bar-archived'
    case 'failed':
    case 'permanently_failed': return 'accent-bar-failed'
    default: return 'accent-bar-draft'
  }
}

function formatDateTime(dateStr: string): string {
  if (!dateStr) return 'NOT SCHEDULED'
  const date = new Date(dateStr)
  if (!Number.isFinite(date.getTime())) return 'DATE UNAVAILABLE'
  return date.toLocaleDateString('en-GB', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit', hour12: false }).toUpperCase().replace(',', ' ·')
}

function formatGigDate(dateStr: string): string {
  return new Date(`${dateStr.slice(0, 10)}T12:00:00Z`).toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric', timeZone: 'UTC' }).toUpperCase()
}

function formatCurrency(amount: number, currency: string): string {
  if (amount == null || String(amount).trim() === '' || !Number.isFinite(Number(amount)) || !currency) return 'FEE UNAVAILABLE'
  try {
    return new Intl.NumberFormat('de-DE', { style: 'currency', currency }).format(Number(amount))
  } catch {
    return 'FEE UNAVAILABLE'
  }
}

const upcomingGig = computed(() => upcomingGigs.value[0] ?? null)

// Computed: social queue (top 3 posts)
const socialQueue = computed(() => {
  return socialPosts.value.slice(0, 3).map(post => ({
    ...post,
    badgeClass: statusBadgeClass(post.status),
    accentClass: statusAccentClass(post.status),
    failed: ['failed', 'permanently_failed'].includes(post.status),
    platform: post.postType === 'story' ? 'INSTAGRAM STORY' : 'INSTAGRAM FEED',
    time: formatDateTime(post.scheduledAtUtc),
  }))
})

// Computed: recent tracklists (top 3 by createdAt desc)
const recentTracklists = computed(() => {
  return [...pastTracklists.value].sort((a, b) => b.createdAt.localeCompare(a.createdAt)).slice(0, 3)
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
      <div v-if="loading || gigError" class="glass hud-card" style="padding:32px 24px;text-align:center;" role="status">{{ loading ? 'LOADING GIGS…' : 'GIGS UNAVAILABLE' }}</div>
      <div v-else-if="upcomingGig" class="glass hud-card glow-card accent-bar-ready" style="display:flex;overflow:hidden;" aria-label="Upcoming gig">
        <div style="flex:1;padding:16px 18px;min-width:0;">
          <!-- Badges -->
          <div style="display:flex;gap:8px;margin-bottom:10px;flex-wrap:wrap;">
            <span v-if="demoData" class="badge-hud badge-draft" data-demo-label>DEMO DATA</span>
            <span class="badge-hud badge-ready" style="border-color:color-mix(in srgb, var(--color-primary) 50%, transparent);box-shadow:0 0 12px rgba(150,248,255,.1);">
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
              <span style="font-family:var(--font-terminal);font-size:9px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-on-surface);">{{ formatGigDate(upcomingGig.date) }}</span>
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
          style="display:flex;flex-direction:column;gap:6px;padding:14px 16px;border-left:1px solid color-mix(in srgb, var(--color-primary) 8%, transparent);justify-content:center;flex-shrink:0;"
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
      <div
        v-else
        aria-label="Upcoming gig"
        class="glass hud-card"
        style="padding:32px 24px 28px;text-align:center;border:1px dashed color-mix(in srgb, var(--color-primary) 20%, transparent);"
      >
        <!-- Icon well: quiet on purpose, so the button is the one thing to find -->
        <div v-if="demoData" class="section-lbl" data-demo-label>DEMO DATA</div>
        <div
          style="width:56px;height:56px;margin:0 auto 16px;display:flex;align-items:center;justify-content:center;border:1px dashed color-mix(in srgb, var(--color-primary) 30%, transparent);background:color-mix(in srgb, var(--color-primary) 5%, transparent);color:var(--color-tertiary);"
        >
          <CalendarPlus style="width:24px;height:24px;" aria-hidden="true" />
        </div>
        <div style="font-family:var(--font-command);font-size:20px;font-weight:700;letter-spacing:-.02em;text-transform:uppercase;color:var(--color-on-surface);">{{ gigs.length ? 'NO UPCOMING GIGS' : 'NO GIGS YET' }}</div>
        <div style="font-family:var(--font-data);font-size:12px;line-height:1.55;color:var(--color-on-surface-variant);max-width:320px;margin:6px auto 0;">Create your first gig to see it here.</div>
        <NuxtLink to="/gigs" class="btn-hud btn-hud-cta luminous-threshold" style="margin-top:20px;padding:0 22px;">
          <Plus style="width:12px;height:12px;flex-shrink:0;" aria-hidden="true" />
          NEW GIG
        </NuxtLink>
      </div>

      <!-- ── Widget grid: 3 columns on desktop ── -->
      <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:14px;" class="col-grid">
        <!-- Column 1: Social Queue -->
        <section aria-label="Social queue" style="display:flex;flex-direction:column;gap:7px;">
          <div v-if="demoData" class="section-lbl" data-demo-label>DEMO DATA</div>
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:2px;">
            <div class="section-lbl">SOCIAL QUEUE</div>
            <NuxtLink to="/social" class="quiet" style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary);cursor:pointer;text-decoration:none;">VIEW ALL →</NuxtLink>
          </div>

          <div v-if="loading || socialError" class="glass accent-bar-draft" style="padding:16px;text-align:center;" role="status">{{ loading ? 'LOADING SOCIAL QUEUE…' : 'SOCIAL QUEUE UNAVAILABLE' }}</div>
          <template v-else-if="socialQueue.length > 0">
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
              <div class="spost-meta" :style="post.failed ? 'color:var(--color-error)' : ''">
                <svg v-if="!post.failed" width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
                <svg v-else width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/></svg>
                {{ post.failed ? (post.lastError || 'PUBLISHING FAILED') : post.time }}
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
          <div v-if="demoData" class="section-lbl" data-demo-label>DEMO DATA</div>
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:2px;">
            <div class="section-lbl">TRACKLISTS</div>
            <NuxtLink to="/tracklist" class="quiet" style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary);cursor:pointer;text-decoration:none;">VIEW ALL →</NuxtLink>
          </div>

          <div v-if="loading || tracklistError" class="glass accent-bar-draft" style="padding:16px;text-align:center;" role="status">{{ loading ? 'LOADING TRACKLISTS…' : 'TRACKLISTS UNAVAILABLE' }}</div>
          <template v-else-if="recentTracklists.length > 0">
            <NuxtLink
              v-for="tl in recentTracklists"
              :key="tl.id"
              to="/tracklist"
              class="glass accent-bar-draft"
              style="display:flex;align-items:center;justify-content:space-between;padding:10px 13px;cursor:pointer;transition:all .15s;text-decoration:none;"
            >
              <div>
                <div style="font-family:var(--font-command);font-size:11px;font-weight:600;color:var(--color-on-surface);text-transform:uppercase;letter-spacing:-.02em;">{{ tl.title }}</div>
                <div style="display:flex;gap:6px;margin-top:3px;">
                  <span class="data-frag">{{ formatDateTime(tl.createdAt) }}</span>
                </div>
              </div>
              <span class="badge-hud badge-draft">SAVED</span>
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
          <div v-if="demoData" class="section-lbl" data-demo-label>DEMO DATA</div>
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:2px;">
            <div class="section-lbl">FINANCE</div>
            <NuxtLink to="/finance" class="quiet" style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary);cursor:pointer;text-decoration:none;">VIEW ALL →</NuxtLink>
          </div>

          <div v-if="financeUnavailable" class="glass accent-bar-draft" style="padding:16px;text-align:center;">{{ loading ? 'LOADING EARNINGS…' : 'EARNINGS UNAVAILABLE' }}</div>
          <template v-else>
          <div class="glass accent-bar-ready" style="padding:12px 14px;">
            <div class="section-lbl" style="margin-bottom:4px;">EARNED THIS MONTH</div>
            <div style="font-family:var(--font-command);font-size:22px;font-weight:700;color:var(--color-primary);letter-spacing:-.02em;text-shadow:0 0 30px rgba(150,248,255,.2);">{{ finance.mtd }}</div>
          </div>

          <div class="glass accent-bar-draft" style="padding:12px 14px;">
            <div class="section-lbl" style="margin-bottom:4px;">PENDING</div>
            <div style="font-family:var(--font-command);font-size:22px;font-weight:700;color:var(--color-secondary);letter-spacing:-.02em;">{{ pendingUnavailable ? 'PENDING UNAVAILABLE' : finance.pending }}</div>
            <div class="spost-meta" style="margin-top:3px;">
              <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
              Outstanding issued invoices
            </div>
          </div>

          <div class="glass" style="padding:12px 14px;">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px;">
              <div class="section-lbl">YTD TOTAL</div>
              <TrendingUp style="width:12px;height:12px;color:var(--color-tertiary);opacity:.5;" />
            </div>
            <div style="font-family:var(--font-command);font-size:18px;font-weight:700;color:var(--color-on-surface);letter-spacing:-.02em;">{{ finance.ytd }}</div>
          </div>
          </template>
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
    border-top: 1px solid color-mix(in srgb, var(--color-primary) 8%, transparent) !important;
  }
}
</style>