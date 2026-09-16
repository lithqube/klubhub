import { cp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = fileURLToPath(new URL('../../', import.meta.url));
const output = path.join(root, 'dist/site');
await rm(output, { recursive: true, force: true });
await mkdir(output, { recursive: true });
await cp(path.join(root, 'apps/site/public'), output, { recursive: true });
// Public assets include valid, OFL-licensed Latin font subsets for the site.
// Extract the application's tokens at build time: no second palette to maintain.
const source = await readFile(path.join(root, 'apps/dj/app/assets/css/styles.css'), 'utf8');
const tokens = source.match(/@theme\s*\{([\s\S]*?)\n\}/)?.[1];
const fonts = [...source.matchAll(/@font-face\s*\{[\s\S]*?\}/g)].map(([rule]) => rule.replaceAll("url('/fonts/", "url('./fonts/").replace('font-style: oblique 0deg 10deg;', 'font-style: normal;'));
if (!tokens || fonts.length !== 3) throw new Error('Application theme format changed; review site token extraction.');
await writeFile(path.join(output, 'theme.css'), `${fonts.join('\n')}\n:root {${tokens}\n}\n`);
console.log('Built static KlubHub site: dist/site');
