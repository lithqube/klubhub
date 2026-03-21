import { describe, it, expect } from 'vitest'
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
  it('shows amber expiry warning when token expires within 7 days', () => {
    const account = makeAccount({ tokenExpiry: expiryDateFromNow(3), status: 'connected' })
    const wrapper = mount(SocialTokenWarningBanner, { props: { account } })
    expect(wrapper.find('.banner').exists()).toBe(true)
    expect(wrapper.text()).toMatch(/EXPIRING IN 3 DAYS/)
    expect(wrapper.find('.banner').classes().join(' ')).toContain('border-amber')
    expect(wrapper.find('.banner').classes().join(' ')).not.toContain('border-secondary')
  })

  it('shows magenta disconnected banner when status is disconnected', () => {
    const account = makeAccount({ status: 'disconnected' })
    const wrapper = mount(SocialTokenWarningBanner, { props: { account } })
    expect(wrapper.find('.banner').exists()).toBe(true)
    expect(wrapper.text()).toMatch(/DISCONNECTED/)
    expect(wrapper.find('.banner').classes().join(' ')).toContain('border-secondary')
    expect(wrapper.find('.banner').classes().join(' ')).not.toContain('border-amber')
  })

  it('renders nothing when token expires in 30 days', () => {
    const account = makeAccount({ tokenExpiry: expiryDateFromNow(30), status: 'connected' })
    const wrapper = mount(SocialTokenWarningBanner, { props: { account } })
    expect(wrapper.find('.banner').exists()).toBe(false)
  })

  it('renders nothing when tokenExpiry is null', () => {
    const account = makeAccount({ tokenExpiry: null, status: 'connected' })
    const wrapper = mount(SocialTokenWarningBanner, { props: { account } })
    expect(wrapper.find('.banner').exists()).toBe(false)
  })
})
