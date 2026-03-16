import { type Browser } from 'playwright-core';

export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id');
  if (!id) throw createError({ statusCode: 400, message: 'id required' });

  const format = (getQuery(event).format as string) ?? 'story';
  const { width, height } =
    format === 'square'
      ? { width: 1080, height: 1080 }
      : { width: 1080, height: 1920 };

  const nitroApp = useNitroApp() as any;
  const browser = nitroApp._playwright as Browser;
  if (!browser)
    throw createError({ statusCode: 503, message: 'Playwright not ready' });

  const context = await browser.newContext({
    viewport: { width, height },
    deviceScaleFactor: 2,
  });
  const page = await context.newPage();

  // Navigate to the Vue PAGE at /render/tracklist/{id} (NOT a Nitro API route)
  // apps/dj/app/pages/render/tracklist/[id].vue handles this URL
  const port = process.env.PORT ?? '3000';
  const renderURL = `http://localhost:${port}/render/tracklist/${id}`;

  await page.goto(renderURL, { waitUntil: 'networkidle', timeout: 15_000 });

  const buffer = await page.screenshot({ type: 'png', fullPage: false });
  await context.close();

  setResponseHeader(event, 'Content-Type', 'image/png');
  return buffer;
});
