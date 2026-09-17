/**
 * Smoke-tests for the static KlubHub site. Each test re-runs
 * `node apps/site/build.mjs` so a clean checkout (no dist/site yet)
 * is exercised end-to-end and a CSS/theme regression cannot pass
 * silently.
 *
 * What is asserted:
 *   - The custom domain + canonical + sitemap + robots + .nojekyll
 *     are published and mutually consistent.
 *   - Every local href resolves on disk and every section id in
 *     index.html is unique.
 *   - The synthesised theme.css exposes the application's design
 *     tokens (--color-surface, --color-primary, --radius) and bundles
 *     exactly three local WOFF2 fonts.
 *   - The marketing copy separates current product (KLUBHUB DJ) from
 *     roadmap products (PROMOTER / LABEL) and ships no third-party
 *     script (the newsletter.js handler is local).
 *   - The newsletter signup wires to Plunk's public track endpoint
 *     with a Plunk public key (pk_*) injected from PLUNK_PUBLIC_KEY
 *     at build time, and the JS handler is shipped as a local static
 *     asset (no third-party script tag, no Mailtrap-shaped residue).
 *   - The build fails closed when PLUNK_PUBLIC_KEY is missing or is
 *     still the placeholder.
 *
 * Run via `node --test apps/site/site.test.mjs` or
 * `pnpm exec nx run site:test`.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile, stat } from 'node:fs/promises';
import { execFileSync, spawnSync } from 'node:child_process';

// A valid-looking but obviously-fake public key for tests. Plunk's track
// endpoint accepts the same shape as a real key; the build only checks
// the prefix and that it isn't the placeholder.
const TEST_KEY = 'pk_test_abcd1234abcd1234abcd1234abcd1234';
const PLACEHOLDER_KEY = 'pk_replace_me_in_your_fork';

execFileSync(process.execPath, ['apps/site/build.mjs'], {
  env: { ...process.env, PLUNK_PUBLIC_KEY: TEST_KEY },
});
const read = (file) => readFile(`dist/site/${file}`, 'utf8');

test('publishes the custom domain and discoverability metadata', async () => {
  assert.equal(await read('CNAME'), 'klubhub.io\n');
  const html = await read('index.html');
  assert.match(html, /rel="canonical" href="https:\/\/klubhub.io\/"/);
  assert.match(await read('sitemap.xml'), /<loc>https:\/\/klubhub.io\/<\/loc>/);
  assert.match(await read('robots.txt'), /Sitemap: https:\/\/klubhub.io\/sitemap.xml/);
  await stat('dist/site/.nojekyll');
});

test('local links resolve and section IDs are unique', async () => {
  const html = await read('index.html');
  const ids = [...html.matchAll(/\bid="([^"]+)"/g)].map((match) => match[1]);
  assert.equal(ids.length, new Set(ids).size);
  for (const [, href] of html.matchAll(/href="([^"]+)"/g)) {
    if (href.startsWith('#')) assert.ok(ids.includes(href.slice(1)), href);
    else if (href.startsWith('./')) await stat(`dist/site/${href.slice(2)}`);
    else assert.ok(href.startsWith('https://'), href);
  }
});

test('uses application design tokens and ships all local fonts', async () => {
  const theme = await read('theme.css');
  assert.match(theme, /--color-surface:\s+#0E0E0F/);
  assert.match(theme, /--color-primary:\s+#96F8FF/);
  assert.match(theme, /--radius:\s+0px/);
  const fonts = [...theme.matchAll(/url\('\.\/(fonts\/[^']+)'\)/g)];
  assert.equal(fonts.length, 3);
  for (const [, font] of fonts) {
    const bytes = await readFile(`dist/site/${font}`);
    assert.equal(bytes.subarray(0, 4).toString(), 'wOF2');
  }
});

test('separates future products and SaaS from available source', async () => {
  const html = await read('index.html');
  for (const name of ['KLUBHUB DJ', 'KLUBHUB PROMOTER', 'KLUBHUB LABEL']) {
    assert.ok(html.includes(name));
  }
  assert.match(html, /No hosted service is available from this site today/);
  assert.match(html, /pricing, and launch date have not been announced/);
});

test('newsletter form is wired to Plunk public-key track endpoint', async () => {
  const html = await read('index.html');
  const js = await read('newsletter.js');

  // HTML wiring — the build injected the exact env-supplied public key.
  assert.match(html, new RegExp(`<meta name="plunk-public-key" content="${TEST_KEY}">`));
  assert.doesNotMatch(html, new RegExp(`content="${PLACEHOLDER_KEY}"`));
  assert.match(html, /<form[^>]*\bdata-newsletter\b/);
  assert.match(html, /<script src="\.\/newsletter\.js" defer>/);
  assert.match(html, /id="newsletter-email"[^>]*type="email"/);
  assert.match(html, /aria-live="polite"/);

  // JS wiring — POSTs to Plunk's /v1/track with Bearer pk_* and event subscribed,
  // and only treats the response as success when Plunk's documented envelope is
  // present (success === true with a data.contact string). Permissive parsing
  // would mask an upstream regression that returns a non-envelope 200.
  assert.match(js, /https:\/\/next-api\.useplunk\.com\/v1\/track/);
  assert.match(js, /Bearer ' \+ publicKey/);
  assert.match(js, /event:\s*'subscribed'/);
  assert.match(js, /Subscribing\.\.\./);
  assert.match(js, /Subscribed\. We will be in touch/);
  assert.match(js, /body\.success === true/);
  assert.match(js, /body\.data\.contact/);

  // No leftover Mailtrap references anywhere.
  assert.doesNotMatch(js, /mailtrap/i);
  assert.doesNotMatch(html, /mailtrap/i);

  // The newsletter.js file is shipped as a local asset (not a third-party CDN).
  await stat('dist/site/newsletter.js');
  assert.doesNotMatch(html, /<script[^>]+src="https?:/);
});

test('build fails closed when PLUNK_PUBLIC_KEY is missing', () => {
  const result = spawnSync(process.execPath, ['apps/site/build.mjs'], {
    env: {
      ...process.env,
      PLUNK_PUBLIC_KEY: '',
      PLUNK_PUBLIC_KEY_STAGING: '',
      PLUNK_PUBLIC_KEY_PREVIEW: '',
    },
    encoding: 'utf8',
  });
  assert.notEqual(result.status, 0, 'build should fail when key is missing');
  assert.match(
    result.stderr + result.stdout,
    /PLUNK_PUBLIC_KEY is required to build the KlubHub site/,
  );
});

test('build fails closed when PLUNK_PUBLIC_KEY is the placeholder', () => {
  const result = spawnSync(process.execPath, ['apps/site/build.mjs'], {
    env: { ...process.env, PLUNK_PUBLIC_KEY: PLACEHOLDER_KEY },
    encoding: 'utf8',
  });
  assert.notEqual(result.status, 0, 'build should fail when key is placeholder');
  assert.match(
    result.stderr + result.stdout,
    /PLUNK_PUBLIC_KEY is still the placeholder value/,
  );
});

test('build fails closed when key is the wrong prefix (sk_*)', () => {
  const result = spawnSync(process.execPath, ['apps/site/build.mjs'], {
    env: { ...process.env, PLUNK_PUBLIC_KEY: 'sk_secret_leak' },
    encoding: 'utf8',
  });
  assert.notEqual(result.status, 0, 'build should fail when key is secret-prefixed');
  assert.match(result.stderr + result.stdout, /must start with `pk_`/);
});

// The CI workflow (`.github/workflows/pages.yml`) injects this exact
// placeholder on PR builds because PR runs cannot read environment
// secrets. The build must accept it (it does) — this test pins the
// placeholder so a rename of the workflow string surfaces as a CI
// failure rather than a silent "PR build fails closed" regression.
test('PR preview placeholder is accepted by the build', () => {
  const result = spawnSync(process.execPath, ['apps/site/build.mjs'], {
    env: {
      ...process.env,
      PLUNK_PUBLIC_KEY: 'pk_ci_pr_preview_only_do_not_subscribe',
      PLUNK_PUBLIC_KEY_STAGING: '',
      PLUNK_PUBLIC_KEY_PREVIEW: '',
    },
    encoding: 'utf8',
  });
  assert.equal(result.status, 0, 'PR preview placeholder must build cleanly');
});
