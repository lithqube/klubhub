import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import PaymentForm from '../PaymentForm.vue'
import { useInvoiceStore, FinanceApiError } from '../../../stores/invoice'
import { makeInvoice } from './fixtures'

function setup(invOverrides = {}, payments = []) {
  const store = useInvoiceStore()
  const wrapper = mount(PaymentForm, {
    props: { invoice: makeInvoice({ outstanding_minor: 50000, received_minor: 20000, ...invOverrides }), payments },
  })
  const save = () => wrapper.findAll('button').find((b) => b.text() === 'SAVE' || b.text() === 'SAVING…')!
  return { store, wrapper, save }
}

describe('PaymentForm', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('keeps SAVE disabled until the amount is valid', async () => {
    const { wrapper, save } = setup()
    expect(save().attributes('disabled')).toBeDefined()
    await wrapper.get('#inv-pf-amount').setValue('200')
    expect(save().attributes('disabled')).toBeUndefined()
  })

  it('announces an amount above the outstanding balance and blocks SAVE', async () => {
    const { wrapper, save } = setup()
    await wrapper.get('#inv-pf-amount').setValue('600')
    const msg = wrapper.get('#inv-pf-msg')
    expect(msg.attributes('aria-live')).toBe('polite')
    expect(msg.text()).toContain('At most €500.00')
    expect(wrapper.get('#inv-pf-amount').attributes('aria-invalid')).toBe('true')
    expect(save().attributes('disabled')).toBeDefined()
  })

  it('limits refunds to what was received', async () => {
    const { wrapper } = setup()
    await wrapper.get('input[value="refund"]').setValue(true)
    await wrapper.get('#inv-pf-amount').setValue('250')
    expect(wrapper.get('#inv-pf-msg').text()).toContain('At most €200.00 can be refunded')
  })

  it('FILL REMAINING fills the limit for the kind', async () => {
    const { wrapper } = setup()
    const fill = wrapper.findAll('button').find((b) => b.text() === 'FILL REMAINING')!
    await fill.trigger('click')
    expect((wrapper.get('#inv-pf-amount').element as HTMLInputElement).value).toBe('500.00')
    await wrapper.get('input[value="refund"]').setValue(true)
    await fill.trigger('click')
    expect((wrapper.get('#inv-pf-amount').element as HTMLInputElement).value).toBe('200.00')
  })

  it('defaults to a refund on a paid invoice, where nothing is outstanding', () => {
    const { wrapper } = setup({ status: 'paid', outstanding_minor: 0, received_minor: 119000 })
    expect((wrapper.get('input[value="refund"]').element as HTMLInputElement).checked).toBe(true)
  })

  it('saves "already received" payments as received and emits saved', async () => {
    const { wrapper, store } = setup()
    const created = { id: 'p1', kind: 'payment', amount_minor: 12345, currency: 'EUR' }
    const spy = vi.spyOn(store, 'createPayment').mockResolvedValue(created as never)
    await wrapper.get('input[value="payment"]').setValue(true)
    await wrapper.get('#inv-pf-amount').setValue('123.45')
    await wrapper.get('form').trigger('submit')
    expect(spy).toHaveBeenCalledWith('inv-1', expect.objectContaining({
      kind: 'payment', amount_minor: 12345, received: true, method: 'bank transfer',
    }))
    expect(spy.mock.calls[0]![1].received_at).toMatch(/^\d{4}-\d{2}-\d{2}T00:00:00Z$/)
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('shows exceeds_balance inline after the server refused', async () => {
    const { wrapper, store } = setup()
    vi.spyOn(store, 'createPayment').mockRejectedValue(new FinanceApiError(422, 'exceeds_balance', 'payment exceeds invoice balance'))
    await wrapper.get('#inv-pf-amount').setValue('100')
    await wrapper.get('form').trigger('submit')
    await new Promise((r) => setTimeout(r))
    expect(wrapper.get('#inv-pf-msg').text()).toMatch(/more than the invoice balance allows/)
  })
})
