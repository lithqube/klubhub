/**
 * Wraps `nuxt build` for the KlubHub Promoter frontend with two safety
 * guarantees a raw `nuxt build` does not give us:
 *
 *   1. NUXT_PUBLIC_API_BASE must be set, otherwise the script exits
 *      with a clear message instead of producing an output that
 *      silently assumes `http://127.0.0.1:8080` (the value baked into
 *      the production runtime image, but never what a dev or CI build
 *      should ship).
 *   2. The in-tree mock handlers under apps/promoter/server/api/v1 are
 *      physically moved out of the `server/` tree before Nitro
 *      discovers routes, then restored in a `finally` block (including
 *      the cross-filesystem cp+rm fallback for Docker BuildKit
 *      overlays). This means a stale cached build cannot re-bundle
 *      the mocks, and a crash mid-build still leaves the working
 *      tree in a recoverable state.
 *
 * Run via `node apps/promoter/scripts/build-prod.mjs` or
 * `pnpm exec nx run @dev/promoter:build`. Used by apps/promoter/Dockerfile.
 *
 * @returns {void} Exits the process with Nuxt's status on success
 *   (`process.exitCode` set on failure).
 */
import { cpSync, existsSync, mkdirSync, renameSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'

/**
 * Run the wrapped Nuxt production build, staging and restoring the
 * in-tree mock handlers around the build so Nitro cannot bundle them.
 * Returns nothing; sets `process.exitCode` on any failure.
 *
 * @returns {void}
 */
function main() {
  const appRoot = join(dirname(fileURLToPath(import.meta.url)), '..')
  const repoRoot = join(appRoot, '..', '..')
  const mockSource = join(appRoot, 'server', 'api', 'v1')
  const mockStaged = join(appRoot, '.mock-api-v1')
  const nuxtCache = join(repoRoot, 'node_modules', '.cache', 'nuxt')
  const output = join(appRoot, '.output')

  if (!process.env.NUXT_PUBLIC_API_BASE) {
    console.error(
      'Refusing production build without NUXT_PUBLIC_API_BASE. ' +
        'Set it to the Go API base, e.g. http://api:8080.',
    )
    process.exit(1)
  }

  if (!existsSync(mockSource)) {
    console.error(`Mock source directory is missing: ${mockSource}`)
    process.exit(1)
  }
  if (existsSync(mockStaged)) {
    console.error(
      `Staging directory already exists: ${mockStaged}\n` +
        `Recover with: mv "${mockStaged}" "${mockSource}"`,
    )
    process.exit(1)
  }

  /**
   * Move `mockSource` aside so Nitro route discovery does not bundle it.
   * On filesystems where rename(2) cannot cross devices (Docker BuildKit
   * overlay), falls back to a recursive copy + delete.
   *
   * @returns {boolean} `true` when the mocks are now staged and the build
   *   can proceed; `false` when the staging failed unrecoverably.
   */
  function stageMocks() {
    try {
      renameSync(mockSource, mockStaged)
    } catch (err) {
      if (err && (err.code === 'EXDEV' || err.code === 'EPERM' || err.code === 'ENOTSUP')) {
        cpSync(mockSource, mockStaged, { recursive: true })
        rmSync(mockSource, { recursive: true, force: true })
      } else {
        console.error(
          `[plan-d] Cannot stage mocks out of server/: ${err && err.code ? err.code : err}`,
        )
        return false
      }
    }
    return true
  }

  /**
   * Restore the staged mocks back under apps/promoter/server/api/v1 once the
   * Nuxt build has finished (success or failure). Same cross-fs fallback
   * as stageMocks.
   *
   * @returns {boolean} `true` when the mocks are restored (or there was
   *   nothing to restore); `false` when restore failed and the operator
   *   must move the directory back by hand.
   */
  function restoreMocks() {
    if (!existsSync(mockStaged)) return true
    if (existsSync(mockSource)) {
      console.error(
        `Cannot restore mocks: both ${mockSource} and ${mockStaged} exist. ` +
          'Manual recovery required.',
      )
      return false
    }
    try {
      renameSync(mockStaged, mockSource)
    } catch (err) {
      if (err && (err.code === 'EXDEV' || err.code === 'EPERM' || err.code === 'ENOTSUP')) {
        mkdirSync(mockSource, { recursive: true })
        cpSync(mockStaged, mockSource, { recursive: true })
        rmSync(mockStaged, { recursive: true, force: true })
      } else {
        console.error(
          `[plan-d] Cannot restore mocks into server/: ${err && err.code ? err.code : err}. ` +
            'Manual recovery required (mv .mock-api-v1 server/api/v1).',
        )
        return false
      }
    }
    console.warn('[plan-d] Restored development/staging mocks')
    return true
  }

  // This must happen before the Nuxt process starts. Config hooks run
  // after Nitro's route discovery and cannot reliably prevent mock
  // bundling, so we move the handlers out of the `server/` tree and put
  // them back once the build is over.
  if (!stageMocks()) {
    process.exitCode = 1
    return
  }

  try {
    // Remove generated route manifests so a prior development build
    // cannot reintroduce cached mock handlers into the production
    // artifact.
    rmSync(nuxtCache, { recursive: true, force: true })
    rmSync(output, { recursive: true, force: true })

    console.warn('[plan-d] Mocks staged outside server/ before Nuxt startup')
    const result = spawnSync('pnpm', ['exec', 'nuxt', 'build'], {
      cwd: appRoot,
      env: { ...process.env, NODE_ENV: 'production' },
      stdio: 'inherit',
    })

    if (result.error) {
      console.error(`[plan-d] nuxt build could not be spawned: ${result.error.message}`)
      process.exitCode = 1
    } else if (result.status !== 0) {
      process.exitCode = result.status ?? 1
    }
  } finally {
    if (!restoreMocks()) {
      process.exitCode = process.exitCode || 1
    }
  }
}

main()
