import { existsSync, renameSync, rmSync } from 'node:fs'
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
  renameSync(mockSource, mockStaged)
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
      renameSync(mockStaged, mockSource)
      console.warn('[plan-d] Restored development/staging mocks')
    }
  }
}
