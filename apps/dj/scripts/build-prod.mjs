/**
 * Wraps `nuxt build` for the KlubHub DJ frontend with two safety
 * guarantees a raw `nuxt build` does not give us:
 *
 *   1. NUXT_PUBLIC_API_BASE must be set, otherwise the script exits
 *      with a clear message instead of producing an output that
 *      silently assumes `http://127.0.0.1:8080` (the value baked into
 *      the production runtime image, but never what a dev or CI build
 *      should ship).
 *   2. The in-tree mock handlers under apps/dj/server/api/v1 are
 *      physically moved out of the `server/` tree before Nitro
 *      discovers routes, then restored in a `finally` block (including
 *      the cross-filesystem cp+rm fallback for Docker BuildKit
 *      overlays). This means a stale cached build cannot re-bundle
 *      the mocks, and a crash mid-build still leaves the working
 *      tree in a recoverable state.
 *
 * Run via `node apps/dj/scripts/build-prod.mjs` or
 * `pnpm exec nx run @dev/dj:build`. Used by apps/dj/Dockerfile.
 *
 * @returns {void} Exits the process with Nuxt's status on success
 *   (`process.exitCode` set on failure). Re-throws on
 *   non-recoverable staging errors so the wrapper surfaces them.
 */
import { copyFileSync, cpSync, existsSync, mkdirSync, renameSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'

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

let staged = false
try {
  // This must happen before the Nuxt process starts. Config hooks run after
  // Nitro's route discovery and cannot reliably prevent mock bundling.
  // The source lives inside the workspace tree (which `COPY . .` makes
  // part of the build's overlay filesystem) and the destination is
  // alongside it — `rename(2)` is only atomic on the same filesystem. In
  // a Docker BuildKit overlay the two paths can live on different layers
  // (EXDEV). Fall back to a recursive copy + delete so the script works
  // in both `pnpm nx build` and `docker build` invocations.
  try {
    renameSync(mockSource, mockStaged)
  } catch (err) {
    if (err && (err.code === 'EXDEV' || err.code === 'EPERM' || err.code === 'ENOTSUP')) {
      cpSync(mockSource, mockStaged, { recursive: true })
      rmSync(mockSource, { recursive: true, force: true })
    } else {
      throw err
    }
  }
  staged = true

  // Remove generated route manifests so a prior development build cannot
  // reintroduce cached mock handlers into the production artifact.
  rmSync(nuxtCache, { recursive: true, force: true })
  rmSync(output, { recursive: true, force: true })

  console.warn('[plan-d] Mocks staged outside server/ before Nuxt startup')
  const result = spawnSync('pnpm', ['exec', 'nuxt', 'build'], {
    cwd: appRoot,
    env: { ...process.env, NODE_ENV: 'production' },
    stdio: 'inherit',
  })

  if (result.error) {
    throw result.error
  }
  if (result.status !== 0) {
    process.exitCode = result.status ?? 1
  }
} finally {
  if (staged && existsSync(mockStaged)) {
    if (existsSync(mockSource)) {
      console.error(
        `Cannot restore mocks: both ${mockSource} and ${mockStaged} exist. ` +
          'Manual recovery required.',
      )
      process.exitCode = 1
    } else {
      try {
        renameSync(mockStaged, mockSource)
      } catch (err) {
        if (err && (err.code === 'EXDEV' || err.code === 'EPERM' || err.code === 'ENOTSUP')) {
          mkdirSync(mockSource, { recursive: true })
          cpSync(mockStaged, mockSource, { recursive: true })
          rmSync(mockStaged, { recursive: true, force: true })
        } else {
          throw err
        }
      }
      console.warn('[plan-d] Restored development/staging mocks')
    }
  }
}
