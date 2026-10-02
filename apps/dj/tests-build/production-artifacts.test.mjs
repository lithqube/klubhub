/** Run after the normal build: node --test apps/dj/tests-build/production-artifacts.test.mjs */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { join, relative } from 'node:path'

const output = fileURLToPath(new URL('../.output/', import.meta.url))
function walk(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const path = join(dir, entry.name)
    return entry.isDirectory() ? walk(path) : entry.isFile() ? [path] : []
  })
}

test('ALL normal production output artifacts exclude fixture seeds and the demo import root', () => {
  const files = walk(output)
  assert.ok(files.length > 0, 'must scan an actual build, not an empty directory')
  const seeds = /club alpha|beta warehouse|cyber void/i
  const demoDependency = /00\.demo\.client|app\/demo\/|klubhub-demo:v1|startDemo/
  const hits = files.flatMap(path => {
    const content = readFileSync(path).toString('utf8')
    return seeds.test(content) || demoDependency.test(content) ? [relative(output, path)] : []
  })
  console.log(`Scanned ${files.length} output files (including server, client, maps and manifests); fixture/demo dependency hits: ${hits.length}`)
  assert.deepEqual(hits, [], 'normal production must not emit the demo dependency or seed chunk')
})
