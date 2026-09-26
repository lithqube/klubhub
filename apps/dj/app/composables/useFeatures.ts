import { resolveFeatureFlags, type FeatureFlags, type FeatureKey } from '~/utils/features'

/**
 * Edition feature flags (see ~/utils/features.ts and docs/EDITIONS.md).
 *
 * Flags come from runtimeConfig.public.features and are read-only at
 * runtime. Every licensed feature is off unless the deployment enables it,
 * e.g. NUXT_PUBLIC_FEATURES_RA_IMPORT=true.
 */
export function useFeatures(): {
  flags: FeatureFlags
  isEnabled: (key: FeatureKey) => boolean
} {
  let raw: unknown
  try {
    raw = (useRuntimeConfig().public as Record<string, unknown>).features
  } catch {
    // Outside a Nuxt app context (e.g. a unit test without a stub): fail closed.
    raw = undefined
  }
  const flags = resolveFeatureFlags(raw)
  return {
    flags,
    isEnabled: (key: FeatureKey) => flags[key] === true,
  }
}
