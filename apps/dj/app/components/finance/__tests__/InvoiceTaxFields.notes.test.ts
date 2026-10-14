// The legal note follows the VAT treatment, using the wording the server
// returns for the supplier's country (GET /invoices/tax-notes), so a German
// supplier never keeps "§ 19 UStG" on an invoice that charges VAT.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import InvoiceTaxFields from '../InvoiceTaxFields.vue'
import { useInvoiceStore } from '../../../stores/invoice'
import { emptyParty } from '../../../utils/invoiceDisplay'
import type { InvoiceTaxFieldsValue } from '../../../types/finance'

const DE = {
  exempt: 'Gemäß § 19 UStG wird keine Umsatzsteuer berechnet. / VAT is not charged under § 19 UStG.',
  reverse_charge: 'Steuerschuldnerschaft des Leistungsempfängers (Reverse Charge). / Reverse charge.',
  outside_scope: 'Nicht steuerbare sonstige Leistung (§ 3a Abs. 2 UStG).',
}

function setup(value: InvoiceTaxFieldsValue, notes: typeof DE | null = DE) {
  setActivePinia(createPinia())
  const store = useInvoiceStore()
  store.taxNotes = notes
  const fetchNotes = vi.spyOn(store, 'fetchTaxNotes').mockResolvedValue()
  const wrapper = mount(InvoiceTaxFields, {
    attachTo: document.body,
    props: { modelValue: value, customer: { ...emptyParty(), country: 'FR' } },
  })
  return { wrapper, fetchNotes }
}

async function pick(wrapper: ReturnType<typeof mount>, treatment: string): Promise<InvoiceTaxFieldsValue> {
  const select = wrapper.get('select')
  ;(select.element as HTMLSelectElement).value = treatment
  await select.trigger('change')
  const events = wrapper.emitted('update:modelValue')!
  return events[events.length - 1]![0] as InvoiceTaxFieldsValue
}

const tax = (over: Partial<InvoiceTaxFieldsValue> = {}): InvoiceTaxFieldsValue =>
  ({ vat_treatment: 'domestic', tax_rate_bps: 1900, tax_note: '', withholding_rate_bps: 0, ...over })

describe('InvoiceTaxFields legal note', () => {
  beforeEach(() => { document.body.innerHTML = '' })

  it('asks for the supplier country\'s wording when it appears', () => {
    const { fetchNotes } = setup(tax())
    expect(fetchNotes).toHaveBeenCalledTimes(1)
  })

  it('applies the country\'s wording when the treatment changes to one that needs a note', async () => {
    const { wrapper } = setup(tax())
    expect((await pick(wrapper, 'exempt')).tax_note).toBe(DE.exempt)
    expect((await pick(wrapper, 'reverse_charge')).tax_note).toBe(DE.reverse_charge)
  })

  it('removes the § 19 wording again when VAT is charged', async () => {
    const { wrapper } = setup(tax({ vat_treatment: 'exempt', tax_rate_bps: 0, tax_note: DE.exempt }))
    const next = await pick(wrapper, 'domestic')
    expect(next.tax_note).toBe('')
    expect(next.vat_treatment).toBe('domestic')
  })

  it('leaves wording the user wrote alone', async () => {
    const own = 'Befreit nach Rücksprache mit dem Finanzamt.'
    const { wrapper } = setup(tax({ vat_treatment: 'exempt', tax_rate_bps: 0, tax_note: own }))
    expect((await pick(wrapper, 'domestic')).tax_note).toBe(own)
  })

  it('works before the notes have loaded, with the built-in wording', async () => {
    const { wrapper } = setup(tax(), null)
    expect((await pick(wrapper, 'exempt')).tax_note).toBe('VAT exempt: small business scheme.')
  })
})
