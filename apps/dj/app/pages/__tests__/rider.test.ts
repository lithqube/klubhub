import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h, reactive } from 'vue'
import RiderPage from '../rider.vue'
import { useRiderAutosave } from '../../composables/useRiderAutosave'
import { useRiderStore } from '../../stores/rider'
import type { RiderAttachment, RiderTemplate } from '../../types/rider'

// The /rider page against a fake API (only $fetch is stubbed; the components,
// store and autosave are real). Covers load/error/empty states, selection,
// the deep link, and what happens when the user leaves.

const NOW = '2026-10-02T12:00:00.000Z'
const tpl = (id: string, name: string, over: Partial<RiderTemplate> = {}): RiderTemplate => ({
  id, name, technical: `${name} technical`, hospitality: '', backline: '', otherNotes: '', createdAt: NOW, updatedAt: NOW, ...over,
})
const att = (gigId: string, over: Partial<RiderAttachment> = {}): RiderAttachment => ({
  id: `att-${gigId}`, gigId, templateId: null, technical: 'attachment technical', hospitality: '', backline: '', otherNotes: '',
  createdAt: NOW, updatedAt: NOW, ...over,
})
const http = (statusCode: number, data?: unknown) => Object.assign(new Error(`HTTP ${statusCode}`), { statusCode, data })
function gate() {
  let open!: () => void
  const promise = new Promise<void>((r) => { open = r })
  return { promise, open }
}

interface Api {
  templates: RiderTemplate[]
  attachments: Record<string, RiderAttachment | null>
  /** When set, GET /templates waits for it. */
  templatesGate?: Promise<void>
  /** When set, GET /attachments/by-gig/* waits for it. */
  attachmentGate?: Promise<void>
  failTemplates?: unknown
  failAttachment?: unknown
}
let api: Api
let calls: string[]

const NuxtLink = defineComponent({
  props: { to: { type: String, default: '' } },
  setup: (p, { slots }) => () => h('a', { href: p.to, 'data-nuxt-link': '' }, slots.default?.()),
})
const route = reactive({ query: {} as Record<string, string> })

let n = 0
let wrapper: VueWrapper | undefined
const usedTargets: { kind: 'template' | 'attachment'; id: string }[] = []

beforeEach(() => {
  n += 1
  calls = []
  api = { templates: [tpl(`t${n}a`, 'Alpha'), tpl(`t${n}b`, 'Bravo')], attachments: {} }
  route.query = {}
  setActivePinia(createPinia())
  vi.stubGlobal('useHead', vi.fn())
  vi.stubGlobal('useRoute', () => route)
  vi.stubGlobal('confirm', vi.fn().mockReturnValue(true))
  vi.stubGlobal('$fetch', vi.fn(async (url: string, o?: { method?: string; body?: Record<string, unknown> }) => {
    const method = o?.method ?? 'GET'
    calls.push(`${method} ${url}`)
    if (url === '/api/v1/rider/templates' && method === 'GET') {
      if (api.templatesGate) await api.templatesGate
      if (api.failTemplates) throw api.failTemplates
      return { data: api.templates.map(t => ({ ...t })) }
    }
    if (url === '/api/v1/rider/templates' && method === 'POST') {
      const created = tpl(`created-${n}`, String(o!.body!.name))
      api.templates.push(created)
      return { data: created }
    }
    const tplMatch = /^\/api\/v1\/rider\/templates\/([^/]+)$/.exec(url)
    if (tplMatch && method === 'DELETE') {
      api.templates = api.templates.filter(t => t.id !== tplMatch[1])
      return undefined
    }
    if (tplMatch && method === 'PUT') {
      const t = api.templates.find(x => x.id === tplMatch[1])!
      const { updatedAt: _token, ...patch } = o!.body!
      Object.assign(t, patch, { updatedAt: new Date(Date.parse(t.updatedAt) + 1000).toISOString() })
      return { data: { ...t } }
    }
    const byGig = /^\/api\/v1\/rider\/attachments\/by-gig\/(.+)$/.exec(url)
    if (byGig) {
      if (api.attachmentGate) await api.attachmentGate
      if (api.failAttachment) throw api.failAttachment
      return { data: api.attachments[byGig[1]!] ?? null }
    }
    throw new Error(`unexpected request: ${method} ${url}`)
  }))
  document.body.replaceChildren()
})
afterEach(async () => {
  wrapper?.unmount()
  wrapper = undefined
  for (const t of usedTargets.splice(0)) await useRiderAutosave().cancel(t)
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

function mountPage() {
  wrapper = mount(RiderPage, {
    attachTo: document.body,
    global: { components: { NuxtLink }, stubs: { RiderAttachmentList: { template: '<div data-testid="attachments-stub" />' } } },
  })
  return wrapper
}
const find = (id: string) => wrapper!.find(`[data-testid="${id}"]`)
const nameInput = () => find('rider-template-name-input').element as HTMLInputElement

describe('/rider templates tab', () => {
  it('shows a loading note, then the list with the first template selected and its editor open', async () => {
    const g = gate()
    api.templatesGate = g.promise
    mountPage()
    await flushPromises()
    expect(find('rider-templates-loading').exists()).toBe(true)
    expect(find('rider-template-name-input').exists()).toBe(false)

    g.open()
    await flushPromises()
    expect(find('rider-templates-loading').exists()).toBe(false)
    expect(wrapper!.findAll('[data-testid^="rider-template-row-"]').map(r => r.text())).toEqual(['Alpha', 'Bravo'])
    expect(nameInput().value).toBe('Alpha')
    expect(find(`rider-template-row-${api.templates[0]!.id}`).attributes('aria-current')).toBe('true')
    usedTargets.push({ kind: 'template', id: api.templates[0]!.id })
  })

  it('does NOT mount an editor from the cache while fresh data is loading (stale text must not be saved under a new token)', async () => {
    const stale = tpl(api.templates[0]!.id, 'Alpha', { technical: 'STALE cached text', updatedAt: '2026-01-01T00:00:00.000Z' })
    useRiderStore().templates = [stale]
    api.templates[0] = tpl(api.templates[0]!.id, 'Alpha', { technical: 'FRESH server text', updatedAt: NOW })
    const g = gate()
    api.templatesGate = g.promise

    mountPage()
    await flushPromises()
    expect(find('rider-template-name-input').exists()).toBe(false) // cached template exists, but no editor yet

    g.open()
    await flushPromises()
    const technical = find('rider-technical-textarea').element as HTMLTextAreaElement
    expect(technical.value).toBe('FRESH server text')
    usedTargets.push({ kind: 'template', id: stale.id })
  })

  it('a failed load shows the reason and a working RETRY', async () => {
    api.failTemplates = http(500, { error: 'internal_error', message: 'an internal error occurred' })
    mountPage()
    await flushPromises()
    expect(find('rider-templates-error').text()).toContain('an internal error occurred')
    expect(find('rider-template-name-input').exists()).toBe(false)

    api.failTemplates = undefined
    await find('rider-templates-retry').trigger('click')
    await flushPromises()
    expect(find('rider-templates-error').exists()).toBe(false)
    expect(nameInput().value).toBe('Alpha')
    usedTargets.push({ kind: 'template', id: api.templates[0]!.id })
  })

  it('with no templates it explains what to do; creating one selects it', async () => {
    api.templates = []
    mountPage()
    await flushPromises()
    expect(wrapper!.text()).toContain('Create a template to reuse the same rider on many gigs.')

    await find('rider-new-template').trigger('click')
    await flushPromises()
    await wrapper!.find('[data-testid="rider-template-dialog-name"]').setValue('Festival')
    await wrapper!.find('[data-testid="rider-template-dialog-submit"]').trigger('click')
    await flushPromises()

    expect(find('rider-template-dialog').exists()).toBe(false) // closed
    expect(nameInput().value).toBe('Festival') // and the new template is open for editing
    usedTargets.push({ kind: 'template', id: `created-${n}` })
  })

  it('creating a template selects it even when others exist', async () => {
    mountPage()
    await flushPromises()
    expect(nameInput().value).toBe('Alpha')

    await find('rider-new-template').trigger('click')
    await flushPromises()
    await wrapper!.find('[data-testid="rider-template-dialog-name"]').setValue('Zulu')
    await wrapper!.find('[data-testid="rider-template-dialog-submit"]').trigger('click')
    await flushPromises()

    expect(nameInput().value).toBe('Zulu')
    usedTargets.push({ kind: 'template', id: api.templates[0]!.id }, { kind: 'template', id: `created-${n}` })
  })

  it('deleting the open template selects the next one; deleting the last shows the empty state', async () => {
    mountPage()
    await flushPromises()
    const [a, b] = api.templates.map(t => t.id)
    expect(nameInput().value).toBe('Alpha')

    await find('rider-template-delete').trigger('click')
    await flushPromises()
    expect(calls).toContain(`DELETE /api/v1/rider/templates/${a}`)
    expect(wrapper!.findAll('[data-testid^="rider-template-row-"]').map(r => r.text())).toEqual(['Bravo'])
    expect(nameInput().value).toBe('Bravo')

    await find('rider-template-delete').trigger('click')
    await flushPromises()
    expect(calls).toContain(`DELETE /api/v1/rider/templates/${b}`)
    expect(find('rider-template-name-input').exists()).toBe(false)
    expect(wrapper!.text()).toContain('Create a template to reuse the same rider on many gigs.')
  })

  it('switching templates swaps the editor to the other template\'s text', async () => {
    mountPage()
    await flushPromises()
    await find(`rider-template-row-${api.templates[1]!.id}`).trigger('click')
    await flushPromises()
    expect(nameInput().value).toBe('Bravo')
    expect((find('rider-technical-textarea').element as HTMLTextAreaElement).value).toBe('Bravo technical')
    usedTargets.push({ kind: 'template', id: api.templates[0]!.id }, { kind: 'template', id: api.templates[1]!.id })
  })

  it('stacks on a phone and splits from md up (layout hooks are in place)', async () => {
    mountPage()
    await flushPromises()
    expect(wrapper!.find('.rider-split').exists()).toBe(true)
    expect(wrapper!.find('.rider-sidebar').exists()).toBe(true)
    expect(wrapper!.find('.rider-main').exists()).toBe(true)
    usedTargets.push({ kind: 'template', id: api.templates[0]!.id })
  })

  it('the tabs are exposed as tabs', async () => {
    mountPage()
    await flushPromises()
    expect(find('rider-tabs').attributes('role')).toBe('tablist')
    expect(find('rider-tab-templates').attributes('aria-selected')).toBe('true')
    await find('rider-tab-attachments').trigger('click')
    expect(find('rider-tab-attachments').attributes('aria-selected')).toBe('true')
    expect(find('rider-tab-templates').attributes('aria-selected')).toBe('false')
    expect(wrapper!.find('[data-testid="attachments-stub"]').exists()).toBe(true)
    usedTargets.push({ kind: 'template', id: api.templates[0]!.id })
  })
})

describe('/rider?gig=<id> deep link', () => {
  const GIG = 'gig-deep'

  it('shows a loading note, then that gig\'s editor; offers a way back', async () => {
    route.query = { gig: GIG }
    api.attachments[GIG] = att(GIG)
    const g = gate()
    api.attachmentGate = g.promise
    mountPage()
    await flushPromises()
    expect(find('rider-deep-link-loading').exists()).toBe(true)
    expect(find('rider-technical-textarea').exists()).toBe(false)
    expect(find('rider-back').attributes('href')).toBe('/rider')

    g.open()
    await flushPromises()
    expect(find('rider-deep-link-loading').exists()).toBe(false)
    expect((find('rider-technical-textarea').element as HTMLTextAreaElement).value).toBe('attachment technical')
    usedTargets.push({ kind: 'attachment', id: `att-${GIG}` })
  })

  it('does NOT open the editor from a stale cached copy: it waits for the refreshed one', async () => {
    route.query = { gig: GIG }
    useRiderStore().attachmentsByGigId = { [GIG]: att(GIG, { technical: 'STALE cached text', updatedAt: '2026-01-01T00:00:00.000Z' }) }
    api.attachments[GIG] = att(GIG, { technical: 'FRESH server text' })
    const g = gate()
    api.attachmentGate = g.promise

    mountPage()
    await flushPromises()
    expect(find('rider-technical-textarea').exists()).toBe(false) // cached, but not shown

    g.open()
    await flushPromises()
    expect((find('rider-technical-textarea').element as HTMLTextAreaElement).value).toBe('FRESH server text')
    usedTargets.push({ kind: 'attachment', id: `att-${GIG}` })
  })

  it('a failed load shows the server\'s reason (e.g. a malformed id) and RETRY works', async () => {
    route.query = { gig: 'not-a-uuid' }
    api.failAttachment = http(400, { error: 'bad_request', message: 'invalid gig id' })
    mountPage()
    await flushPromises()
    expect(find('rider-deep-link-error').text()).toContain('invalid gig id')
    expect(find('rider-technical-textarea').exists()).toBe(false)

    api.failAttachment = undefined
    api.attachments['not-a-uuid'] = att('not-a-uuid')
    await find('rider-deep-link-retry').trigger('click')
    await flushPromises()
    expect(find('rider-deep-link-error').exists()).toBe(false)
    expect(find('rider-technical-textarea').exists()).toBe(true)
    usedTargets.push({ kind: 'attachment', id: 'att-not-a-uuid' })
  })

  it('a gig with no rider says so and offers to attach one, instead of a dead end', async () => {
    route.query = { gig: GIG }
    api.attachments[GIG] = null
    mountPage()
    await flushPromises()
    expect(find('rider-deep-link-empty').text()).toContain('No rider is attached to this gig yet.')
    expect(find(`rider-attach-btn-${GIG}`).exists()).toBe(true)
    expect(find('rider-technical-textarea').exists()).toBe(false)
  })

  it('changing the link while a load is in flight does not show the first gig\'s result', async () => {
    route.query = { gig: 'first' }
    api.attachments.first = att('first', { technical: 'FIRST' })
    api.attachments.second = att('second', { technical: 'SECOND' })
    const g = gate()
    api.attachmentGate = g.promise
    mountPage()
    await flushPromises()

    route.query = { gig: 'second' }
    await flushPromises()
    g.open()
    await flushPromises()
    expect((find('rider-technical-textarea').element as HTMLTextAreaElement).value).toBe('SECOND')
    usedTargets.push({ kind: 'attachment', id: 'att-first' }, { kind: 'attachment', id: 'att-second' })
  })
})

describe('/rider when the user leaves', () => {
  const beforeUnload = () => {
    const e = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(e)
    return e
  }

  it('warns before closing or reloading while edits are still queued, and not otherwise', async () => {
    vi.useFakeTimers()
    mountPage()
    await vi.advanceTimersByTimeAsync(50)
    await flushPromises()
    expect(beforeUnload().defaultPrevented).toBe(false) // nothing edited

    await find('rider-technical-textarea').setValue('typed a moment ago')
    expect(beforeUnload().defaultPrevented).toBe(true)

    await vi.advanceTimersByTimeAsync(2000) // autosave fires and succeeds
    await flushPromises()
    expect(beforeUnload().defaultPrevented).toBe(false)
    usedTargets.push({ kind: 'template', id: api.templates[0]!.id })
  })

  it('pushes queued edits out when the tab is hidden', async () => {
    vi.useFakeTimers()
    mountPage()
    await vi.advanceTimersByTimeAsync(50)
    await flushPromises()
    await find('rider-technical-textarea').setValue('about to switch tabs')
    expect(calls.filter(c => c.startsWith('PUT'))).toHaveLength(0)

    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true })
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })

    expect(calls.filter(c => c.startsWith('PUT'))).toEqual([`PUT /api/v1/rider/templates/${api.templates[0]!.id}`])
    usedTargets.push({ kind: 'template', id: api.templates[0]!.id })
  })

  it('does nothing when the tab becomes visible again', async () => {
    vi.useFakeTimers()
    mountPage()
    await vi.advanceTimersByTimeAsync(50)
    await flushPromises()
    await find('rider-technical-textarea').setValue('still typing')
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(calls.filter(c => c.startsWith('PUT'))).toHaveLength(0)
    usedTargets.push({ kind: 'template', id: api.templates[0]!.id })
  })

  it('stops listening once the page is gone', async () => {
    vi.useFakeTimers()
    mountPage()
    await vi.advanceTimersByTimeAsync(50)
    await flushPromises()
    await find('rider-technical-textarea').setValue('edit')
    wrapper!.unmount()
    wrapper = undefined
    await flushPromises() // the unmount flush saves it
    expect(beforeUnload().defaultPrevented).toBe(false)
    usedTargets.push({ kind: 'template', id: api.templates[0]!.id })
  })
})
