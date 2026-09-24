// @vitest-environment node
import { existsSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const root = new URL('../../../../../', import.meta.url)

function project(path: string) {
  const file = fileURLToPath(new URL(path, root))
  expect(existsSync(file), `${path} must define real verification targets`).toBe(true)
  return JSON.parse(readFileSync(file, 'utf8'))
}

describe('verification target contracts', () => {
  it('gates on a Phase 5-scoped real typecheck plus the Go race suite', () => {
    const target = project('apps/dj/project.json').targets.typecheck
    expect(target, 'an explicit real typecheck target is required').toBeDefined()
    expect(target.executor).toBe('nx:run-commands')
    expect(target.cache).toBe(false)
    expect(target.options.command).toBe('bash scripts/typecheck-phase5.sh')
    expect(target.options.cwd).toBe('.')
    expect(target.options.envFile).toBe('{workspaceRoot}/.env.example')
  })
  it('runs uncached API race tests from the Go module without skipping integration', () => {
    const target = project('api/project.json').targets.test
    expect(target.executor).toBe('nx:run-commands')
    expect(target.cache).toBe(false)
    expect(target.options.cwd).toBe('api')
    expect(target.options.command).toBe('go test ./... -race -count=1 -p 1')
  })
})
