import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useBillingProfileStore } from '../billingProfile'
import { FinanceApiError } from '../invoice'
import { DEFAULT_BILLING_PROFILE } from '../../../shared/finance-mock/rules'
import type { UpdateBillingProfileRequest } from '../../types/finance'

const fetchMock = vi.fn()
const httpError = (status: number, body: unknown) => Object.assign(new Error(`HTTP ${status}`), { statusCode: status, data: body })

beforeEach(() => {
  setActivePinia(createPinia())
  fetchMock.mockReset()
  vi.stubGlobal('$fetch', fetchMock)
})

describe('billing profile store', () => {
  it('loads the profile from the data envelope', async () => {
    fetchMock.mockResolvedValue({ data: { ...DEFAULT_BILLING_PROFILE, tax_number: '21/815/08150', iban: 'DE89370400440532013000', bic: 'COBADEFFXXX' } })
    const store = useBillingProfileStore()
    await store.fetchProfile()
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/finance/billing-profile')
    expect(store.profile).toMatchObject({ tax_number: '21/815/08150', iban: 'DE89370400440532013000', bic: 'COBADEFFXXX' })
  })

  it('reports a missing profile as a not_found error', async () => {
    fetchMock.mockRejectedValue(httpError(404, { error: 'not_found', message: 'billing profile not found' }))
    const store = useBillingProfileStore()
    expect(await store.fetchProfile()).toBeNull()
    expect(store.error?.code).toBe('not_found')
  })

  it('PUTs tax_number, iban and bic and adopts the server-normalised response', async () => {
    const { id: _id, created_at: _c, ...rest } = DEFAULT_BILLING_PROFILE
    const body: UpdateBillingProfileRequest = { ...rest, tax_number: '21/815/08150', iban: 'de89 3704 0044 0532 0130 00', bic: 'cobadeffxxx' }
    fetchMock.mockResolvedValue({ data: { ...DEFAULT_BILLING_PROFILE, ...body, iban: 'DE89370400440532013000', bic: 'COBADEFFXXX', updated_at: '2026-09-25T11:00:00Z' } })
    const store = useBillingProfileStore()
    await store.updateProfile(body)

    const [url, opts] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/finance/billing-profile')
    expect(opts.method).toBe('PUT')
    expect(opts.body).toMatchObject({ tax_number: '21/815/08150', iban: 'de89 3704 0044 0532 0130 00', bic: 'cobadeffxxx', updated_at: DEFAULT_BILLING_PROFILE.updated_at })
    expect(store.profile?.iban).toBe('DE89370400440532013000')
  })

  it('throws a typed error on 400 and keeps the previous profile', async () => {
    const store = useBillingProfileStore()
    store.profile = { ...DEFAULT_BILLING_PROFILE }
    fetchMock.mockRejectedValue(httpError(400, { error: 'validation_failed', message: 'validation failed: iban: not a valid IBAN (check the country, length and check digits)' }))
    const { id: _id, created_at: _c, ...rest } = DEFAULT_BILLING_PROFILE
    const err = await store.updateProfile({ ...rest, iban: 'DE00' }).catch((e) => e)
    expect(err).toBeInstanceOf(FinanceApiError)
    expect(err.field).toBe('iban')
    expect(store.profile?.iban).toBe('')
  })
})
