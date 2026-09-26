import { describe, it, expect } from 'vitest'
import { FEATURES, FEATURE_KEYS, resolveFeatureFlags } from '../features'

describe('edition feature registry', () => {
  it('declares every feature with a paid edition and both env vars', () => {
    expect(FEATURE_KEYS.length).toBeGreaterThan(0)
    for (const key of FEATURE_KEYS) {
      const def = FEATURES[key]
      expect(['licensed', 'saas']).toContain(def.edition)
      expect(def.description.length).toBeGreaterThan(0)
      expect(def.apiEnv).toMatch(/^FEATURE_[A-Z0-9_]+$/)
      expect(def.appEnv).toMatch(/^NUXT_PUBLIC_FEATURES_[A-Z0-9_]+$/)
    }
  })

  it('declares RA import as a licensed feature', () => {
    expect(FEATURES.raImport.edition).toBe('licensed')
    expect(FEATURES.raImport.apiEnv).toBe('FEATURE_RA_IMPORT')
    expect(FEATURES.raImport.appEnv).toBe('NUXT_PUBLIC_FEATURES_RA_IMPORT')
  })
})

describe('resolveFeatureFlags', () => {
  it('turns every feature off when config is missing', () => {
    for (const raw of [undefined, null, 'yes', 42, {}]) {
      const flags = resolveFeatureFlags(raw)
      for (const key of FEATURE_KEYS) expect(flags[key]).toBe(false)
    }
  })

  it('only accepts true or "true" as on', () => {
    expect(resolveFeatureFlags({ raImport: true }).raImport).toBe(true)
    expect(resolveFeatureFlags({ raImport: 'true' }).raImport).toBe(true)
    expect(resolveFeatureFlags({ raImport: 1 }).raImport).toBe(false)
    expect(resolveFeatureFlags({ raImport: 'false' }).raImport).toBe(false)
  })

  it('ignores unknown keys and returns a frozen map', () => {
    const flags = resolveFeatureFlags({ raImport: true, somethingElse: true })
    expect(Object.keys(flags)).toEqual(FEATURE_KEYS)
    expect(Object.isFrozen(flags)).toBe(true)
  })
})
