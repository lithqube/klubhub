import { afterEach, describe, expect, it } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import InvoiceLinesEditor from '../InvoiceLinesEditor.vue'
import { emptyLine, type LineDraft } from '../../../utils/invoiceLines'

let wrapper: VueWrapper | undefined
afterEach(() => { wrapper?.unmount(); wrapper = undefined; document.body.replaceChildren() })

function row(over: Partial<LineDraft> = {}): LineDraft {
  return { ...emptyLine(), description: 'DJ set', quantity: '1', unit: '80', ...over }
}

/** Mounts the editor under a parent that owns the v-model, like the draft editor does. */
function mountEditor(initial: LineDraft[], props: Record<string, unknown> = {}) {
  const model = ref<LineDraft[]>(initial)
  const Parent = defineComponent({
    setup: () => () => h(InvoiceLinesEditor, {
      modelValue: model.value, currency: 'EUR', ...props,
      'onUpdate:modelValue': (v: LineDraft[]) => { model.value = v },
    }),
  })
  wrapper = mount(Parent, { attachTo: document.body })
  return { model, w: wrapper }
}

const q = (sel: string) => document.querySelector(sel) as HTMLInputElement | HTMLSelectElement
async function type(sel: string, value: string) {
  const el = q(sel)
  el.value = value
  el.dispatchEvent(new Event('input', { bubbles: true }))
  await flushPromises()
}
const removeButtons = () => Array.from(document.querySelectorAll<HTMLButtonElement>('button[aria-label^="Remove line"]'))
const addButton = () => Array.from(document.querySelectorAll('button')).find((b) => b.textContent?.includes('ADD LINE')) as HTMLButtonElement

describe('InvoiceLinesEditor', () => {
  it('labels every input and shows a live per-line total from quantity x unit price', async () => {
    mountEditor([row({ quantity: '2', unit: '80.50' })])
    expect(document.querySelector('label[for="invf-lines-0-description"]')).not.toBeNull()
    expect(document.querySelector('label[for="invf-lines-0-quantity"]')).not.toBeNull()
    expect(document.querySelector('label[for="invf-lines-0-unit_code"]')).not.toBeNull()
    expect(document.querySelector('label[for="invf-lines-0-unit_minor"]')).not.toBeNull()
    expect(q('[data-testid="line-total-0"]').textContent).toContain('161.00')

    await type('#invf-lines-0-quantity', '3')
    expect(q('[data-testid="line-total-0"]').textContent).toContain('241.50')
    expect(q('[data-testid="lines-subtotal"]').textContent).toContain('241.50')
  })

  it('says the server recomputes tax and totals', () => {
    mountEditor([row()])
    expect(document.body.textContent).toMatch(/calculated by the server/i)
  })

  it('offers exactly piece, hour, day, lump sum and kilometre and emits the chosen unit', async () => {
    const { model } = mountEditor([row()])
    const select = q('#invf-lines-0-unit_code') as HTMLSelectElement
    expect(Array.from(select.options).map((o) => [o.value, o.textContent?.trim()])).toEqual([
      ['C62', 'Piece'], ['HUR', 'Hour'], ['DAY', 'Day'], ['LS', 'Lump sum'], ['KMT', 'Kilometre'],
    ])
    expect(select.value).toBe('C62')
    select.value = 'HUR'
    select.dispatchEvent(new Event('change', { bubbles: true }))
    await flushPromises()
    expect(model.value[0]!.unit_code).toBe('HUR')
  })

  it('adds a row and moves focus to its description', async () => {
    const { model } = mountEditor([row()])
    addButton().click()
    await flushPromises()
    expect(model.value).toHaveLength(2)
    expect(model.value[1]).toMatchObject({ description: '', quantity: '1', unit: '', unit_code: 'C62' })
    expect(document.activeElement?.id).toBe('invf-lines-1-description')
  })

  it('removes a row, keeps the others, and returns focus to the list', async () => {
    const { model } = mountEditor([row({ description: 'A' }), row({ description: 'B' }), row({ description: 'C' })])
    removeButtons()[1]!.click()
    await flushPromises()
    expect(model.value.map((l) => l.description)).toEqual(['A', 'C'])
    expect(document.activeElement?.id).toBe('invf-lines-1-description')
    expect(q('#invf-lines-1-description').value).toBe('C')
  })

  it('never removes the last remaining line', async () => {
    const { model } = mountEditor([row()])
    const [only] = removeButtons()
    expect(only!.disabled).toBe(true)
    only!.click()
    await flushPromises()
    expect(model.value).toHaveLength(1)
  })

  it('stops adding at the 100 line limit', () => {
    mountEditor(Array.from({ length: 100 }, () => row()))
    expect(addButton().disabled).toBe(true)
  })

  it('shows a local problem only after the field is left, then clears it when fixed', async () => {
    mountEditor([row({ quantity: '1' })])
    expect(document.querySelector('#invf-lines-0-quantity-msg')).toBeNull()
    await type('#invf-lines-0-quantity', '1.5')
    expect(document.querySelector('#invf-lines-0-quantity-msg')).toBeNull()
    q('#invf-lines-0-quantity').dispatchEvent(new Event('blur'))
    await flushPromises()
    expect(q('#invf-lines-0-quantity').getAttribute('aria-invalid')).toBe('true')
    expect(document.querySelector('#invf-lines-0-quantity-msg')?.textContent).toMatch(/whole number/i)
    expect(q('#invf-lines-0-quantity').getAttribute('aria-describedby')).toBe('invf-lines-0-quantity-msg')
    await type('#invf-lines-0-quantity', '4')
    expect(q('#invf-lines-0-quantity').getAttribute('aria-invalid')).toBeNull()
  })

  it('shows every local problem at once when asked to (after a save attempt)', () => {
    mountEditor([row({ description: '', unit: '' })], { showAllProblems: true })
    expect(document.querySelector('#invf-lines-0-description-msg')?.textContent).toMatch(/describe/i)
    expect(document.querySelector('#invf-lines-0-unit_minor-msg')?.textContent).toMatch(/price/i)
  })

  it('maps server errors by line path onto the right inputs, including unit_minor on the price field', () => {
    mountEditor([row(), row()], {
      errors: {
        'lines[1].quantity': 'must be between 1 and 1000000',
        'lines[0].unit_minor': 'must not be negative',
        'lines[1].unit_code': 'must be one of C62, HUR, DAY, LS, KMT',
        lines: 'an invoice needs at least one line',
      },
    })
    expect(q('#invf-lines-1-quantity').getAttribute('aria-invalid')).toBe('true')
    expect(document.querySelector('#invf-lines-1-quantity-msg')?.textContent).toContain('between 1 and 1000000')
    expect(q('#invf-lines-0-quantity').getAttribute('aria-invalid')).toBeNull()
    expect(q('#invf-lines-0-unit_minor').getAttribute('aria-invalid')).toBe('true')
    expect(document.querySelector('#invf-lines-0-unit_minor-msg')?.textContent).toContain('must not be negative')
    expect(q('#invf-lines-1-unit_code').getAttribute('aria-invalid')).toBe('true')
    expect(document.querySelector('[role="alert"]')?.textContent).toContain('at least one line')
  })

  it('uses the currency minor-unit exponent for the preview (JPY has none)', () => {
    mountEditor([row({ quantity: '2', unit: '5000' })], { currency: 'JPY' })
    expect(q('[data-testid="line-total-0"]').textContent).toMatch(/10,000/)
    expect(q('[data-testid="line-total-0"]').textContent).not.toMatch(/10,000\.00/)
  })
})
