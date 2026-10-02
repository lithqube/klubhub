// @vitest-environment node
import { afterEach, describe, expect, it } from 'vitest'
import { chmodSync, cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'

const sandboxes: string[] = []
afterEach(() => sandboxes.splice(0).forEach(dir => rmSync(dir, { recursive: true, force: true })))

function sandbox() {
  const root = mkdtempSync(join(process.env.TMPDIR || tmpdir(), 'dj-build-gate-'))
  sandboxes.push(root)
  const app = join(root, 'apps/dj')
  mkdirSync(join(app, 'scripts'), { recursive: true })
  mkdirSync(join(app, 'server/api/v1'), { recursive: true })
  mkdirSync(join(root, 'bin'))
  cpSync(new URL('../scripts/build-prod.mjs', import.meta.url), join(app, 'scripts/build-prod.mjs'))
  writeFileSync(join(app, 'server/api/v1/fixture.ts'), 'original mock source')
  const executable = join(root, 'bin/pnpm')
  writeFileSync(executable, `#!${process.execPath}\nrequire('node:fs').writeFileSync(process.env.HARNESS_OUTPUT, JSON.stringify(process.env)); process.exit(23)\n`)
  chmodSync(executable, 0o755)
  return { root, app, script: join(app, 'scripts/build-prod.mjs') }
}

describe('normal production build target', () => {
  it('rejects an explicit demo target instead of silently producing demo output', () => {
    const { root, script } = sandbox()
    const result = spawnSync(process.execPath, [script], {
      env: { ...process.env, NUXT_DEMO: '1', HARNESS_OUTPUT: join(root, 'env.json'), NUXT_PUBLIC_API_BASE: 'http://api:8080', PATH: `${join(root, 'bin')}:${process.env.PATH}` },
      encoding: 'utf8',
    })
    expect(result.status).toBe(1)
    expect(result.stderr).toContain('build-demo.mjs')
  })

  it('forces production stage and restores all staged sources on a failed Nuxt process', () => {
    const { root, app, script } = sandbox()
    const output = join(root, 'env.json')
    const result = spawnSync(process.execPath, [script], {
      env: { ...process.env, NUXT_DEMO: '0', NUXT_DEPLOYMENT_STAGE: 'staging', NUXT_PUBLIC_API_BASE: 'http://api:8080', HARNESS_OUTPUT: output, PATH: `${join(root, 'bin')}:${process.env.PATH}` },
      encoding: 'utf8',
    })
    expect(result.status).toBe(23)
    const env = JSON.parse(readFileSync(output, 'utf8'))
    expect(env.NODE_ENV).toBe('production')
    expect(env.NUXT_DEPLOYMENT_STAGE).toBe('production')
    expect(env.NUXT_DEMO).toBe('0')
    expect(readFileSync(join(app, 'server/api/v1/fixture.ts'), 'utf8')).toBe('original mock source')
    expect(existsSync(join(app, '.mock-api-v1'))).toBe(false)
  })
})
