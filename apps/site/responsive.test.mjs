/**
 * Browser layout regression for the built static site (no signup requests).
 * Run `pnpm exec playwright install chromium` once, then
 * `pnpm nx run site:test-responsive`. Kept separate from the dependency-free
 * site:test used by the GitHub Pages workflow.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { chromium } from '@playwright/test';

execFileSync(process.execPath, ['apps/site/build.mjs'], {
  env: { ...process.env, PLUNK_PUBLIC_KEY: 'pk_test_responsive_do_not_subscribe' },
});

const siteUrl = new URL('../../dist/site/index.html', import.meta.url).href;

test('newsletter remains readable and its controls fit at every breakpoint', async (t) => {
  const browser = await chromium.launch();
  try {
    for (const width of [320, 375, 390, 650, 651, 768, 950, 951, 1280, 1440]) {
      await t.test(`${width}px`, async () => {
        const page = await browser.newPage({ viewport: { width, height: 900 } });
        try {
          // Never contact Plunk or other external services, even on regression.
          await page.route(/^https?:/, (route) => route.abort());
          await page.goto(siteUrl);
          await page.evaluate(() => document.fonts.ready);
          const layout = await page.evaluate(() => {
            const rect = (selector) => {
              const { x, y, width, height, right, bottom } = document
                .querySelector(selector).getBoundingClientRect();
              return { x, y, width, height, right, bottom };
            };
            return {
              pageWidth: document.documentElement.scrollWidth,
              panel: rect('.newsletter-inner'),
              copy: rect('.newsletter-copy'),
              form: rect('.newsletter-form'),
              input: rect('.newsletter-input'),
              button: rect('.newsletter-submit'),
              inputFontSize: parseFloat(getComputedStyle(document.querySelector('.newsletter-input')).fontSize),
              newsletterScrollWidth: document.querySelector('.newsletter-inner').scrollWidth,
              newsletterClientWidth: document.querySelector('.newsletter-inner').clientWidth,
            };
          });
          t.diagnostic(`${width}px: page=${layout.pageWidth}px, copy=${layout.copy.width}px, button=${layout.button.width}x${layout.button.height}px`);
          assert.ok(layout.pageWidth <= width, 'page must not scroll horizontally');
          if (width <= 650) {
            assert.ok(layout.inputFontSize >= 16,
              'mobile email text must be readable without triggering iOS focus zoom');
          }
          for (const name of ['copy', 'form', 'input', 'button']) {
            const box = layout[name];
            assert.ok(box.x >= layout.panel.x && box.right <= layout.panel.right + 1,
              `${name} must fit inside newsletter panel at ${width}px: ${JSON.stringify(box)}`);
            assert.ok(box.x >= 0 && box.right <= width,
              `${name} must remain on-screen at ${width}px`);
          }
          assert.ok(layout.newsletterScrollWidth <= layout.newsletterClientWidth + 1,
            'newsletter must not overflow horizontally');
          assert.ok(layout.input.height >= 44 && layout.button.height >= 44,
            'form controls must remain usable touch targets');
          if (width <= 950) {
            assert.ok(layout.form.y >= layout.copy.bottom,
              'form must sit below the copy on mobile/tablet, not squeeze it into a column');
            assert.ok(layout.copy.width >= layout.panel.width - 84,
              'stacked copy must use the available panel width');
          } else {
            assert.ok(layout.form.x >= layout.copy.right,
              'desktop keeps copy and form side by side');
          }
          if (width <= 650) {
            assert.ok(layout.button.y >= layout.input.bottom,
              'mobile button must stack below the email field');
            assert.ok(Math.abs(layout.button.width - layout.form.width) <= 1,
              'mobile button must fill the form width');
          } else {
            assert.ok(layout.button.x >= layout.input.right,
              'wider screens keep the email and button inline');
          }
        } finally {
          await page.close();
        }
      });
    }
  } finally {
    await browser.close();
  }
});

test('phones get full-width actions, readable text and a reachable install block', async (t) => {
  const browser = await chromium.launch();
  try {
    for (const width of [320, 375, 390, 650, 651, 1280]) {
      await t.test(`${width}px`, async () => {
        const page = await browser.newPage({ viewport: { width, height: 900 } });
        try {
          await page.route(/^https?:/, (route) => route.abort());
          await page.goto(siteUrl);
          await page.evaluate(() => document.fonts.ready);
          const m = await page.evaluate(() => {
            const box = (el) => el.getBoundingClientRect();
            const visible = (el) => box(el).width > 0 && getComputedStyle(el).visibility !== 'hidden';
            const texts = [...document.querySelectorAll('body *')].filter((el) => visible(el)
              && [...el.childNodes].some((n) => n.nodeType === 3 && n.textContent.trim()));
            return {
              pageWidth: document.documentElement.scrollWidth,
              smallestText: Math.min(...texts.map((el) => parseFloat(getComputedStyle(el).fontSize))),
              smallTargets: [...document.querySelectorAll('a.button, button, .brand, nav a, .text-link')]
                .filter(visible).filter((el) => box(el).height < 44).map((el) => el.textContent.trim().slice(0, 24)),
              heroButtons: [...document.querySelectorAll('.hero .actions .button')].map((el) => box(el).width),
              djButtons: [...document.querySelectorAll('.dj-intro .button')].map((el) => ({ x: box(el).x, width: box(el).width })),
              install: box(document.getElementById('install')),
              command: box(document.getElementById('install-command')),
              copy: box(document.querySelector('.install .copy')),
              navInstall: !!document.querySelector('nav a[href="#install"]'),
            };
          });
          t.diagnostic(`${width}px: smallest text ${m.smallestText}px, hero buttons ${m.heroButtons.map(Math.round).join('/')}`);
          assert.ok(m.pageWidth <= width, 'page must not scroll horizontally');
          assert.ok(m.navInstall, 'the header links to the install block');
          assert.ok(m.smallestText >= 10, `no text below 10px (found ${m.smallestText}px)`);
          assert.deepEqual(m.smallTargets, [], 'links and buttons stay at least 44px tall');
          assert.ok(m.command.right <= m.install.right + 1 && m.copy.right <= m.install.right + 1,
            'install command and copy button fit inside the install block');
          assert.ok(m.copy.height >= 44, 'copy button is a usable touch target');
          if (width <= 650) {
            const [first, ...rest] = m.heroButtons;
            assert.ok(rest.every((w) => Math.abs(w - first) <= 1), 'hero buttons share one full width on phones');
            assert.ok(m.djButtons.every((b) => Math.abs(b.x - m.djButtons[0].x) <= 1 && Math.abs(b.width - m.djButtons[0].width) <= 1),
              'DJ panel buttons stack aligned, without a leftover desktop offset');
            assert.ok(m.copy.y >= m.command.bottom, 'copy button stacks under the command on phones');
          }
        } finally {
          await page.close();
        }
      });
    }
  } finally {
    await browser.close();
  }
});
