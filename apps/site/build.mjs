/**
 * Builds the static KlubHub GitHub Pages site (apps/site) into
 * dist/site. Copies the public/ tree as-is, then synthesizes a
 * theme.css from the application's @theme tokens and @font-face
 * rules so the marketing site ships with the exact same palette and
 * fonts as the KlubHub DJ product — no second palette to maintain.
 *
 * Exits non-zero if the @theme block cannot be located, the number of
 * @font-face rules does not match the expected count, or the Plunk
 * public key is missing or still the placeholder, so a CSS refactor
 * that silently drops either will fail the build instead of shipping
 * a half-themed page.
 *
 * Run via `node apps/site/build.mjs` or `pnpm exec nx run site:build`.
 *
 * Required env when building for production, staging, or preview
 * (anything that ships to a public URL):
 *   PLUNK_PUBLIC_KEY  Plunk public key (`pk_*`). The build fails closed
 *                     if missing or equal to the placeholder. Three
 *                     independent Plunk projects (staging, production,
 *                     preview) keep a leaked staging key from touching
 *                     the production newsletter list.
 *
 * @returns {Promise<void>} Resolves when the dist/site/ output is in
 *   place. Logs a one-line "Built static KlubHub site: dist/site"
 *   summary on success.
 */
import { cp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = fileURLToPath(new URL('../../', import.meta.url));
const output = path.join(root, 'dist/site');
const sourceHtml = path.join(root, 'apps/site/public/index.html');

const PUBLIC_KEY_PLACEHOLDER = 'pk_replace_me_in_your_fork';

await rm(output, { recursive: true, force: true });
await mkdir(output, { recursive: true });
await cp(path.join(root, 'apps/site/public'), output, { recursive: true });

// Inject the Plunk public key. CI passes one of three values:
//   PLUNK_PUBLIC_KEY             — production (klubhub.io)
//   PLUNK_PUBLIC_KEY_STAGING     — staging deployment
//   PLUNK_PUBLIC_KEY_PREVIEW     — preview deployment
// We accept any of the three so the workflow can pick per environment.
// Fail closed if none is set or the placeholder is still in use — the
// site would otherwise ship a broken newsletter that no operator
// notices until the first signup silently disappears.
const publicKey =
  process.env.PLUNK_PUBLIC_KEY ||
  process.env.PLUNK_PUBLIC_KEY_STAGING ||
  process.env.PLUNK_PUBLIC_KEY_PREVIEW ||
  '';

if (!publicKey) {
  throw new Error(
    'PLUNK_PUBLIC_KEY is required to build the KlubHub site. ' +
      'Set PLUNK_PUBLIC_KEY (or _STAGING / _PREVIEW) to a Plunk public ' +
      'key (pk_*). See docs/SELF-HOSTING.md and the KlubHub GitHub ' +
      'Pages workflow for environment-specific secrets.',
  );
}
if (publicKey === PUBLIC_KEY_PLACEHOLDER) {
  throw new Error(
    'PLUNK_PUBLIC_KEY is still the placeholder value. Replace it with ' +
      'a real Plunk public key (pk_*) from the matching Plunk project ' +
      '(staging / production / preview).',
  );
}
if (!publicKey.startsWith('pk_')) {
  throw new Error(
    'PLUNK_PUBLIC_KEY must start with `pk_` (Plunk public key). ' +
      'Secret keys (sk_*) must never be shipped to a static site.',
  );
}

const html = await readFile(sourceHtml, 'utf8');
const rendered = html.replace(
  /<meta name="plunk-public-key" content="[^"]*">/,
  `<meta name="plunk-public-key" content="${publicKey}">`,
);
if (rendered === html) {
  throw new Error(
    'Plunk public-key meta tag not found in apps/site/public/index.html. ' +
      'The build expects the placeholder tag and replaces it; a CSS or ' +
      'HTML refactor that removes it must add it back so this build can ' +
      'succeed.',
  );
}
await writeFile(path.join(output, 'index.html'), rendered);

// Public assets include valid, OFL-licensed Latin font subsets for the site.
// Extract the application's tokens at build time: no second palette to maintain.
const themeSource = await readFile(
  path.join(root, 'libs/ui/app/assets/css/kinetic.css'),
  'utf8',
);
const tokens = themeSource.match(/@theme\s*\{([\s\S]*?)\n\}/)?.[1];
const fonts = [...themeSource.matchAll(/@font-face\s*\{[\s\S]*?\}/g)].map(
  ([rule]) =>
    rule
      .replaceAll("url('/fonts/", "url('./fonts/")
      .replace('font-style: oblique 0deg 10deg;', 'font-style: normal;'),
);
if (!tokens || fonts.length !== 3) {
  throw new Error('Application theme format changed; review site token extraction.');
}
await writeFile(
  path.join(output, 'theme.css'),
  `${fonts.join('\n')}\n:root {${tokens}\n}\n`,
);

console.log('Built static KlubHub site: dist/site');
