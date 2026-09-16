import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile, stat } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';

execFileSync(process.execPath, ['apps/site/build.mjs']);
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
  for (const name of ['KLUBHUB DJ', 'KLUBHUB PROMOTER', 'KLUBHUB LABEL']) assert.ok(html.includes(name));
  assert.match(html, /No hosted service is available from this site today/);
  assert.match(html, /pricing, and launch date have not been announced/);
  assert.doesNotMatch(html, /<script|<form/);
});
