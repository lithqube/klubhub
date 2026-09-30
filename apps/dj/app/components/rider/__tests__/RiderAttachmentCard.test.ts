import { describe, it, expect } from 'vitest'
import { riderStatusLabel, type RiderAttachment, type RiderTemplate } from '~/types/rider'

const baseAttachment = (overrides: Partial<RiderAttachment> = {}): RiderAttachment => ({
  id: 'att-1',
  gigId: 'gig-1',
  templateId: 'tmpl-1',
  technical: 't', hospitality: 'h', backline: 'b', otherNotes: 'o',
  createdAt: '2025-09-01T10:00:00Z',
  updatedAt: '2025-09-01T10:00:00Z',
  ...overrides,
})

const baseTemplate = (overrides: Partial<RiderTemplate> = {}): RiderTemplate => ({
  id: 'tmpl-1',
  name: 'Standard club',
  technical: 't', hospitality: 'h', backline: 'b', otherNotes: 'o',
  createdAt: '2025-09-01T10:00:00Z',
  updatedAt: '2025-09-01T10:00:00Z',
  ...overrides,
})

describe('riderStatusLabel', () => {
  it('returns muted tone when attachment is null', () => {
    const result = riderStatusLabel(null, null)
    expect(result.label).toBe('NO RIDER ATTACHED')
    expect(result.tone).toBe('muted')
  })

  it('returns accent tone when attached from template (untouched)', () => {
    const result = riderStatusLabel(baseAttachment(), baseTemplate({ name: 'Standard club' }))
    expect(result.label).toBe('FROM STANDARD CLUB')
    expect(result.tone).toBe('accent')
  })

  it('returns amber tone when per-gig copy was overridden (updatedAt > createdAt)', () => {
    const att = baseAttachment({
      createdAt: '2025-09-01T10:00:00Z',
      updatedAt: '2025-09-02T10:00:00Z',
    })
    const result = riderStatusLabel(att, baseTemplate({ name: 'Standard club' }))
    expect(result.label).toBe('CUSTOM (FROM STANDARD CLUB)')
    expect(result.tone).toBe('amber')
  })

  it('falls back to "TEMPLATE" placeholder when template lookup missing', () => {
    const result = riderStatusLabel(baseAttachment({ templateId: 'tmpl-missing' }), null)
    expect(result.label).toBe('FROM TEMPLATE')
  })
})