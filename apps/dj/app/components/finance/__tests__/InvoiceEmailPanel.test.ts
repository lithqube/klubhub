import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import InvoiceEmailPanel from '../InvoiceEmailPanel.vue'
import { FinanceApiError, useInvoiceStore } from '../../../stores/invoice'
import { emptyParty } from '../../../utils/invoiceDisplay'
import { makeInvoice } from './fixtures'

function setup(answer: unknown, email = 'buchhaltung@club.example') {
  setActivePinia(createPinia())
  const store = useInvoiceStore()
  const spy = vi.spyOn(store, 'sendInvoiceEmail')
  if (answer instanceof Error) spy.mockRejectedValue(answer)
  else spy.mockResolvedValue(answer as never)
  const invoice = makeInvoice({ id: 'inv-9', status: 'issued', customer: { ...emptyParty(), email } })
  const wrapper = mount(InvoiceEmailPanel, { props: { invoice } })
  return { wrapper, spy }
}

const sent = { id: 'm', to_email: 'buchhaltung@club.example', subject: 's', status: 'sent', attachment_ids: ['a', 'b'] }
const open = async (w: ReturnType<typeof mount>) => { await w.get('.iem-toggle').trigger('click') }
const buttonByText = (w: ReturnType<typeof mount>, text: string) => w.findAll('button').find((b) => b.text().startsWith(text))!

describe('InvoiceEmailPanel', () => {
  beforeEach(() => { document.body.innerHTML = '' })

  it('starts closed and prefills the customer address', async () => {
    const { wrapper } = setup(sent)
    expect(wrapper.find('.iem-body').exists()).toBe(false)
    await open(wrapper)
    expect((wrapper.get('input[type=email]').element as HTMLInputElement).value).toBe('buchhaltung@club.example')
  })

  it('needs a second click that names the recipient before anything is sent', async () => {
    const { wrapper, spy } = setup(sent)
    await open(wrapper)
    await buttonByText(wrapper, 'SEND…').trigger('click')
    expect(spy).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('buchhaltung@club.example?')
    await buttonByText(wrapper, 'CANCEL').trigger('click')
    expect(spy).not.toHaveBeenCalled()
  })

  it('sends with the e-invoice by default and reports the files', async () => {
    const { wrapper, spy } = setup(sent)
    await open(wrapper)
    await buttonByText(wrapper, 'SEND…').trigger('click')
    await buttonByText(wrapper, 'SEND NOW').trigger('click')
    await flushPromises()
    expect(spy).toHaveBeenCalledWith('inv-9', { to_email: 'buchhaltung@club.example' })
    expect(wrapper.text()).toContain('Sent to buchhaltung@club.example with 2 files.')
  })

  it('says where replies will go, when the server set a reply-to', async () => {
    const { wrapper } = setup({ ...sent, reply_to: 'lina@dj.example' })
    await open(wrapper)
    await buttonByText(wrapper, 'SEND…').trigger('click')
    await buttonByText(wrapper, 'SEND NOW').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Replies go to lina@dj.example.')
  })

  it('does not mention replies when there is no reply-to', async () => {
    const { wrapper } = setup(sent)
    await open(wrapper)
    await buttonByText(wrapper, 'SEND…').trigger('click')
    await buttonByText(wrapper, 'SEND NOW').trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain('Replies go to')
  })

  it('sends the PDF alone when the box is unticked', async () => {
    const { wrapper, spy } = setup(sent)
    await open(wrapper)
    await wrapper.get('input[type=checkbox]').setValue(false)
    await buttonByText(wrapper, 'SEND…').trigger('click')
    await buttonByText(wrapper, 'SEND NOW').trigger('click')
    await flushPromises()
    expect(spy).toHaveBeenCalledWith('inv-9', { to_email: 'buchhaltung@club.example', include_einvoice: false })
  })

  it('uses an edited recipient', async () => {
    const { wrapper, spy } = setup(sent)
    await open(wrapper)
    await wrapper.get('input[type=email]').setValue('  other@client.example ')
    await buttonByText(wrapper, 'SEND…').trigger('click')
    await buttonByText(wrapper, 'SEND NOW').trigger('click')
    await flushPromises()
    expect(spy).toHaveBeenCalledWith('inv-9', { to_email: 'other@client.example' })
  })

  it('asks for an address instead of sending without one', async () => {
    const { wrapper, spy } = setup(sent, '')
    await open(wrapper)
    await buttonByText(wrapper, 'SEND…').trigger('click')
    expect(spy).not.toHaveBeenCalled()
    expect(wrapper.get('[role=alert]').text()).toContain('Enter an email address')
  })

  it('says a failed delivery is saved for retry, not lost', async () => {
    const { wrapper } = setup({ ...sent, status: 'failed' })
    await open(wrapper)
    await buttonByText(wrapper, 'SEND…').trigger('click')
    await buttonByText(wrapper, 'SEND NOW').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('retried automatically')
  })

  it('says so when the server cannot send mail', async () => {
    const { wrapper } = setup(null)
    await open(wrapper)
    await buttonByText(wrapper, 'SEND…').trigger('click')
    await buttonByText(wrapper, 'SEND NOW').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain("isn't enabled")
    expect(wrapper.find('input[type=email]').exists()).toBe(false)
  })

  it('shows a refusal with its reasons', async () => {
    const err = new FinanceApiError(422, 'not_exportable', 'cannot', [{ field: 'einvoice', message: 'No e-invoice to attach.' }])
    const { wrapper } = setup(err)
    await open(wrapper)
    await buttonByText(wrapper, 'SEND…').trigger('click')
    await buttonByText(wrapper, 'SEND NOW').trigger('click')
    await flushPromises()
    const alert = wrapper.get('[role=alert]').text()
    expect(alert).toContain('cannot')
    expect(alert).toContain('No e-invoice to attach.')
    expect(buttonByText(wrapper, 'SEND…')).toBeTruthy() // can try again
  })
})
