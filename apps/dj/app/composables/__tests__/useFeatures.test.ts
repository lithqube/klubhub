import { describe, it, expect, afterEach, vi } from 'vitest'
import { useFeatures } from '../useFeatures'

function stubFeatures(features: unknown) {
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { features } }))
}

describe('useFeatures', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('reports RA import off with the default runtime config', () => {
    stubFeatures({ raImport: false })
    expect(useFeatures().isEnabled('raImport')).toBe(false)
  })

  it('reports RA import off when the features block is absent', () => {
    vi.stubGlobal('useRuntimeConfig', () => ({ public: {} }))
    expect(useFeatures().isEnabled('raImport')).toBe(false)
  })

  it('fails closed outside a Nuxt context', () => {
    // No useRuntimeConfig global at all.
    expect(useFeatures().isEnabled('raImport')).toBe(false)
  })

  it('reports RA import on when runtime config enables it', () => {
    stubFeatures({ raImport: true })
    const { isEnabled, flags } = useFeatures()
    expect(isEnabled('raImport')).toBe(true)
    expect(flags.raImport).toBe(true)
  })

  it('exposes read-only flags', () => {
    stubFeatures({ raImport: false })
    const { flags } = useFeatures()
    expect(() => {
      ;(flags as Record<string, boolean>).raImport = true
    }).toThrow()
    expect(flags.raImport).toBe(false)
  })
})
