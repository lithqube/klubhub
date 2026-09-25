import { describe, it, expect } from 'vitest';
import {
  buildQuickPreview,
  formatMoney,
  formatDayLabel,
  countdownLabel,
  type PreviewPost,
} from '../quickPreview';
import type { Gig } from '../../types/gig';

// A fixed clock: Thu 24 Sep 2026, mid-afternoon local time.
const NOW = new Date(2026, 8, 24, 15, 30);

let seq = 0;
function gig(over: Partial<Gig>): Gig {
  seq += 1;
  return {
    id: `g${seq}`,
    date: '2026-10-01',
    venue: 'Depot',
    city: 'Berlin',
    country: 'DE',
    event_name: `Night ${seq}`,
    promoter_name: '',
    promoter_email: '',
    promoter_phone: '',
    fee_amount: 1000,
    fee_currency: 'EUR',
    set_length_minutes: 60,
    notes: '',
    status: 'confirmed',
    payment_status: 'unpaid',
    gig_reader_venue_id: null,
    gig_reader_contact_id: null,
    created_at: '',
    updated_at: '',
    deleted_at: null,
    ...over,
  };
}

const build = (gigs: Gig[], posts: PreviewPost[] | null = []) =>
  buildQuickPreview({ gigs, posts, now: NOW });

describe('buildQuickPreview: next gig', () => {
  it('picks the earliest confirmed or advanced gig from today on', () => {
    const p = build([
      gig({ id: 'later', date: '2026-11-02' }),
      gig({ id: 'soon', date: '2026-10-05', status: 'advanced' }),
      gig({ id: 'past', date: '2026-09-01' }),
    ]);
    expect(p.nextGig?.gig.id).toBe('soon');
  });

  it('counts calendar days, and a gig today is 0 days away', () => {
    expect(build([gig({ date: '2026-09-24' })]).nextGig?.days).toBe(0);
    expect(build([gig({ date: '2026-09-25' })]).nextGig?.days).toBe(1);
    expect(build([gig({ date: '2026-10-17T00:00:00Z' })]).nextGig?.days).toBe(23);
  });

  it('ignores inquiries, cancelled and played gigs', () => {
    const p = build([
      gig({ status: 'inquiry', date: '2026-09-30' }),
      gig({ status: 'cancelled', date: '2026-09-30' }),
      gig({ status: 'played', date: '2026-09-30' }),
    ]);
    expect(p.nextGig).toBeNull();
  });

  it('is null with no gigs and skips a gig whose date cannot be read', () => {
    expect(build([]).nextGig).toBeNull();
    expect(build([gig({ date: 'soon' })]).nextGig).toBeNull();
  });
});

describe('buildQuickPreview: money', () => {
  it('sums played gigs of the current month as EARNED MTD, reading string fees', () => {
    const p = build([
      gig({ status: 'played', date: '2026-09-05', fee_amount: '1500' as unknown as number }),
      gig({ status: 'played', date: '2026-09-12', fee_amount: 500 }),
      gig({ status: 'played', date: '2026-08-30', fee_amount: 9999 }),
      gig({ status: 'confirmed', date: '2026-09-28', fee_amount: 700 }),
    ]);
    expect(p.earnedMtd).toEqual({ amount: 2000, currency: 'EUR', mixed: false });
  });

  it('sums upcoming confirmed and advanced gigs as BOOKED, leaving out inquiries', () => {
    const p = build([
      gig({ date: '2026-10-02', fee_amount: 1200 }),
      gig({ date: '2026-10-09', status: 'advanced', fee_amount: 800 }),
      gig({ date: '2026-10-16', status: 'inquiry', fee_amount: 5000 }),
    ]);
    expect(p.booked?.amount).toBe(2000);
  });

  it('is null rather than zero when nothing counts', () => {
    const p = build([gig({ status: 'played', date: '2026-09-01', fee_amount: 0 })]);
    expect(p.earnedMtd).toBeNull();
    expect(p.booked).toBeNull();
  });

  it('shows the largest currency and marks a mixed total', () => {
    const p = build([
      gig({ date: '2026-10-02', fee_amount: 1000, fee_currency: 'EUR' }),
      gig({ date: '2026-10-03', fee_amount: 300, fee_currency: 'GBP' }),
    ]);
    expect(p.booked).toEqual({ amount: 1000, currency: 'EUR', mixed: true });
  });
});

describe('buildQuickPreview: payments', () => {
  it('counts and sums overdue gigs, whatever their status but cancelled', () => {
    const p = build([
      gig({ status: 'played', payment_status: 'overdue', fee_amount: 2000 }),
      gig({ status: 'confirmed', payment_status: 'overdue', fee_amount: 500 }),
      gig({ status: 'cancelled', payment_status: 'overdue', fee_amount: 9000 }),
    ]);
    expect(p.payments.overdueCount).toBe(2);
    expect(p.payments.overdue?.amount).toBe(2500);
  });

  it('counts only played gigs as unpaid, not deposits or upcoming gigs', () => {
    const p = build([
      gig({ status: 'played', payment_status: 'unpaid', fee_amount: 1000 }),
      gig({ status: 'played', payment_status: 'deposit_paid', fee_amount: 1000 }),
      gig({ status: 'played', payment_status: 'paid', fee_amount: 1000 }),
      gig({ status: 'confirmed', payment_status: 'unpaid', fee_amount: 1000 }),
    ]);
    expect(p.payments.unpaidCount).toBe(1);
    expect(p.payments.unpaid?.amount).toBe(1000);
  });

  it('reports nothing outstanding when everything is settled', () => {
    const p = build([gig({ status: 'played', payment_status: 'paid' })]);
    expect(p.payments).toEqual({ overdueCount: 0, overdue: null, unpaidCount: 0, unpaid: null });
  });
});

describe('buildQuickPreview: social queue', () => {
  it('counts scheduled, publishing and failed posts and finds the next slot', () => {
    const p = build([], [
      { status: 'scheduled', scheduledAt: '2026-09-30T18:00:00Z' },
      { status: 'publishing', scheduledAt: '2026-09-26T09:00:00Z' },
      { status: 'failed', scheduledAt: '2026-09-20T09:00:00Z' },
      { status: 'permanently_failed', scheduledAt: null },
      { status: 'draft', scheduledAt: '2026-09-25T09:00:00Z' },
      { status: 'published', scheduledAt: '2026-09-01T09:00:00Z' },
    ]);
    expect(p.social).toEqual({ scheduled: 2, failed: 2, nextAt: '2026-09-26T09:00:00.000Z' });
  });

  it('keeps an unreadable queue (null) apart from an empty one', () => {
    expect(build([], null).social).toBeNull();
    expect(build([], []).social).toEqual({ scheduled: 0, failed: 0, nextAt: null });
  });
});

describe('formatters', () => {
  it('formats whole-euro amounts and marks mixed totals', () => {
    expect(formatMoney({ amount: 1500, currency: 'EUR', mixed: false })).toBe('€1,500');
    expect(formatMoney({ amount: 1500, currency: 'EUR', mixed: true })).toBe('€1,500+');
    expect(formatMoney(null)).toBe('—');
  });

  it('survives a malformed currency code', () => {
    expect(formatMoney({ amount: 1500, currency: 'not-a-code', mixed: false })).toBe('1500 not-a-code');
  });

  it('labels days and countdowns', () => {
    expect(formatDayLabel(new Date(2026, 9, 17))).toBe('OCT 17');
    expect(countdownLabel(0)).toBe('TODAY');
    expect(countdownLabel(1)).toBe('TOMORROW');
    expect(countdownLabel(23)).toBe('IN 23D');
  });
});
