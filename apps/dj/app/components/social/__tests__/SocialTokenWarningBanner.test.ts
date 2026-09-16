import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SocialTokenWarningBanner from '../SocialTokenWarningBanner.vue'
import type { SocialAccount } from '../../../types/social'

function makeAccount(overrides: Partial<SocialAccount> = {}): SocialAccount {
  return {
    id: 'acc-1',
    platform: 'instagram',
    igUserId: 'ig-123',
    accountName: 'test_user',
    tokenExpiry: null,
    status: 'connected',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function expiryDateFromNow(days: number): string {
  const d = new Date()
  d.setDate(d.getDate() + days)
  return d.toISOString()
}

describe('SocialTokenWarningBanner', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-16T12:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())
  it('shows amber expiry warning when token expires within 7 days', () => {
    const account = makeAccount({ tokenExpiry: expiryDateFromNow(3), status: 'connected' })
    const wrapper = mount(SocialTokenWarningBanner, { props: { account } })
    expect(wrapper.find('.glass-panel').exists()).toBe(true)
    expect(wrapper.text()).toMatch(/EXPIRING IN 3 DAYS/)
    expect(wrapper.find('.glass-panel').classes().join(' ')).toContain('border-status-archived')
    expect(wrapper.find('.glass-panel').classes().join(' ')).not.toContain('border-error')
  })

  it('shows error-colored disconnected banner when status is disconnected', () => {
    const account = makeAccount({ status: 'disconnected' })
    const wrapper = mount(SocialTokenWarningBanner, { props: { account } })
    expect(wrapper.find('.glass-panel').exists()).toBe(true)
    expect(wrapper.text()).toMatch(/DISCONNECTED/)
    expect(wrapper.find('.glass-panel').classes().join(' ')).toContain('border-error')
    expect(wrapper.find('.glass-panel').classes().join(' ')).not.toContain('border-status-archived')
  })

  it('renders nothing when token expires in 30 days', () => {
    const account = makeAccount({ tokenExpiry: expiryDateFromNow(30), status: 'connected' })
    const wrapper = mount(SocialTokenWarningBanner, { props: { account } })
    expect(wrapper.find('.glass-panel').exists()).toBe(false)
  })

  it('renders nothing when tokenExpiry is null', () => {
    const account = makeAccount({ tokenExpiry: null, status: 'connected' })
    const wrapper = mount(SocialTokenWarningBanner, { props: { account } })
    expect(wrapper.find('.glass-panel').exists()).toBe(false)
  })
})
