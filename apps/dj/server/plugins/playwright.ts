import { chromium, type Browser } from 'playwright-core';

export default defineNitroPlugin(async (nitroApp) => {
  const browser: Browser = await chromium.launch({
    headless: true,
    args: [
      '--no-sandbox',
      '--disable-setuid-sandbox',
      '--disable-dev-shm-usage',
      '--disable-gpu',
    ],
  });

  (nitroApp as any)._playwright = browser;

  nitroApp.hooks.hookOnce('close', async () => {
    await browser.close();
  });
});
