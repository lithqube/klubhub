/**
 * Builds the browser-only demo (docs/DEMO.md): `nuxt generate` with
 * NUXT_DEMO=1, then copies the static output to dist/demo.
 *
 *   node apps/dj/scripts/build-demo.mjs     (or: pnpm nx run @dev/dj:build-demo)
 *
 * NUXT_APP_BASE_URL overrides the default /demo/ base (e.g. "/" to serve the
 * output from a server root). DEMO_OUT_DIR overrides dist/demo.
 */
import { cpSync, existsSync, rmSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'

const appRoot = join(dirname(fileURLToPath(import.meta.url)), '..')
const repoRoot = join(appRoot, '..', '..')
const generated = join(appRoot, '.output-demo', 'public')
const out = resolve(repoRoot, process.env.DEMO_OUT_DIR || 'dist/demo')

rmSync(join(appRoot, '.output-demo'), { recursive: true, force: true })
const result = spawnSync('pnpm', ['--config.verify-deps-before-run=false', 'exec', 'nuxt', 'generate'], {
  cwd: appRoot,
  env: { ...process.env, NUXT_DEMO: '1', NODE_ENV: 'production' },
  stdio: 'inherit',
})
if (result.error || result.status !== 0) {
  console.error(`[demo] nuxt generate failed${result.error ? `: ${result.error.message}` : ''}`)
  process.exit(result.status || 1)
}
for (const page of ['index.html', '404.html', 'gigs/index.html', 'finance/index.html']) {
  if (!existsSync(join(generated, page))) {
    console.error(`[demo] expected ${page} in ${generated}`)
    process.exit(1)
  }
}
rmSync(out, { recursive: true, force: true })
cpSync(generated, out, { recursive: true })
console.log(`[demo] Built browser-only demo: ${out}`)
