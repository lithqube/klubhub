// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('nuxt/config', () => ({ defineNuxtConfig: (config: unknown) => config }))
beforeEach(() => {
  vi.stubEnv('NUXT_DEPLOYMENT_STAGE', undefined)
  vi.stubEnv('NUXT_PUBLIC_DEMO', undefined)
})
afterEach(() => { vi.unstubAllEnvs(); vi.resetModules() })

describe('dashboard uses the existing mock/proxy build gate', () => {
  it.each([
    ['development', '', '', true],
    ['development', 'http://api:8080', '', false],
    ['staging', '', '', true],
    ['staging', 'http://api:8080', '', false],
    ['prod', '', '', false],
    ['production', 'http://api:8080', '', false],
    ['test', '', '', false],
    ['unknown', '', '', false],
    [undefined, '', '', false],
    ['production', '', '1', true],
  ])('NODE_ENV=%s API=%s DEMO=%s labels=%s', async (env, api, demo, expected) => {
    vi.stubEnv('NODE_ENV', env)
    vi.stubEnv('NUXT_PUBLIC_API_BASE', api)
    vi.stubEnv('NUXT_DEMO', demo)
    vi.resetModules()
    const { default: config } = await import('../nuxt.config')
    expect(config.runtimeConfig.public.dashboardMockData).toBe(expected)
    expect(config.runtimeConfig.public.demo).toBe(demo === '1')
  })

  it.each([
    ['production', 'staging', '', true],
    ['production', 'staging', 'http://api:8080', false],
    ['development', 'production', 'http://api:8080', false],
    ['development', 'test', '', false],
    ['development', 'unknown', '', false],
    ['development', '', '', false],
    ['production', 'development', '', true],
  ])('explicit deployment stage %s/%s overrides compiler mode', async (nodeEnv, stage, api, expected) => {
    vi.stubEnv('NODE_ENV', nodeEnv)
    vi.stubEnv('NUXT_DEPLOYMENT_STAGE', stage)
    vi.stubEnv('NUXT_PUBLIC_API_BASE', api)
    vi.stubEnv('NUXT_DEMO', '0')
    const { default: config } = await import('../nuxt.config')
    expect(config.runtimeConfig.public.dashboardMockData).toBe(expected)
    expect((config.nitro.ignore ?? []).includes('api/v1/**')).toBe(!expected)
  })

  it.each(['1', 'true'])('keeps the explicitly opted-in dedicated demo target %s', async (demo) => {
    vi.stubEnv('NODE_ENV', 'production')
    vi.stubEnv('NUXT_PUBLIC_API_BASE', '')
    vi.stubEnv('NUXT_DEMO', demo)
    const { default: config } = await import('../nuxt.config')
    const app = { plugins: [{ src: '/app/plugins/00.demo.client.ts' }] }
    config.hooks['app:resolve'](app)
    expect(app.plugins).toHaveLength(1)
    expect(config.runtimeConfig.public.dashboardMockData).toBe(true)
    expect(config.ssr).toBe(false)
    expect(config.buildDir).toBe('.nuxt-demo')
    expect(config.nitro.output.dir).toBe('.output-demo')
  })

  it.each(['0', '', 'false'])('excludes the demo dependency root at build time for NUXT_DEMO=%s', async (demo) => {
    vi.stubEnv('NODE_ENV', 'production')
    vi.stubEnv('NUXT_PUBLIC_API_BASE', 'http://api:8080')
    vi.stubEnv('NUXT_DEMO', demo)
    // Public runtime flags must not affect the compiler's plugin selection.
    vi.stubEnv('NUXT_PUBLIC_DEMO', 'true')
    const { default: config } = await import('../nuxt.config')
    const app = { plugins: [
      { src: '/app/plugins/00.demo.client.ts' },
      { src: '/app/plugins/other.client.ts' },
    ] }
    config.hooks?.['app:resolve']?.(app)
    expect(app.plugins).toEqual([{ src: '/app/plugins/other.client.ts' }])
  })

  it('still refuses an ordinary production build with no real backend', async () => {
    vi.stubEnv('NODE_ENV', 'production')
    vi.stubEnv('NUXT_PUBLIC_API_BASE', '')
    vi.stubEnv('NUXT_DEMO', '')
    vi.resetModules()
    await expect(import('../nuxt.config')).rejects.toThrow('Refusing to build a production Nuxt image')
  })
})
