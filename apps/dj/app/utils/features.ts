/**
 * Edition feature registry — the single place paid features are declared.
 *
 * KlubHub DJ ships as one codebase in several editions (see
 * docs/EDITIONS.md). Features that belong to the licensed / SaaS editions
 * are listed here and are OFF unless the deployment switches them on via
 * runtime config:
 *
 *   runtimeConfig.public.features.<key>   (nuxt.config.ts, default false)
 *   NUXT_PUBLIC_FEATURES_<KEY>=true        (env override, e.g.
 *                                          NUXT_PUBLIC_FEATURES_RA_IMPORT)
 *
 * The API gates the matching routes with its own FEATURE_* env vars; both
 * sides must be enabled for a feature to work end to end.
 *
 * To add a feature: add an entry below, add `<key>: false` to
 * runtimeConfig.public.features in nuxt.config.ts, gate the UI with
 * `useFeatures().isEnabled('<key>')`, and document it in docs/EDITIONS.md.
 */

export type Edition = 'open-source' | 'licensed' | 'saas'

export interface FeatureDefinition {
  /** Lowest edition the feature ships in ('licensed' also covers SaaS). */
  edition: Exclude<Edition, 'open-source'>
  /** Human-readable summary for docs and admin tooling (never shown to DJs when off). */
  description: string
  /** API env var that must also be true for the backend routes to exist. */
  apiEnv: string
  /** Nuxt env var that flips the runtime-config flag. */
  appEnv: string
}

export const FEATURES = {
  raImport: {
    edition: 'licensed',
    description: 'Resident Advisor integration: import artist bio/links into the EPK and upcoming events into the gig tracker.',
    apiEnv: 'FEATURE_RA_IMPORT',
    appEnv: 'NUXT_PUBLIC_FEATURES_RA_IMPORT',
  },
} as const satisfies Record<string, FeatureDefinition>

export type FeatureKey = keyof typeof FEATURES

export type FeatureFlags = Readonly<Record<FeatureKey, boolean>>

export const FEATURE_KEYS = Object.keys(FEATURES) as FeatureKey[]

/**
 * Normalises raw runtime-config values into a complete, frozen flag map.
 * Unknown keys are ignored; anything other than `true` / `'true'` is off,
 * so a missing or malformed value can never switch a paid feature on.
 */
export function resolveFeatureFlags(raw: unknown): FeatureFlags {
  const source = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const flags = {} as Record<FeatureKey, boolean>
  for (const key of FEATURE_KEYS) {
    const value = source[key]
    flags[key] = value === true || value === 'true'
  }
  return Object.freeze(flags)
}
