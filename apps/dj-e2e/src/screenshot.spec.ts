import { test, expect } from '@playwright/test';

// E2E tests require a running stack (docker compose up -d)
// These tests are skipped in unit test runs — they run only in full e2e context
// PNG dimensions are read directly from the IHDR chunk — no external dependencies needed.
// PNG IHDR: bytes 8-15 = signature, bytes 16-19 = width (big-endian), bytes 20-23 = height (big-endian)

test.describe('Screenshot pipeline', () => {
  test('screenshot dimensions — story (1080×1920 at 2x = 2160×3840)', async ({
    request,
  }) => {
    // This test requires a real tracklist to exist in the DB
    // For now: create a tracklist via API, then call screenshot endpoint
    // TODO in execution: use a pre-seeded test tracklist ID from the DB
    test.skip(true, 'Requires running Docker stack with seeded data');

    const res = await request.get(
      '/api/screenshot/tracklist/test-id?format=story',
    );
    expect(res.status()).toBe(200);
    expect(res.headers()['content-type']).toBe('image/png');
    const body = await res.body();
    const buffer = Buffer.from(body);
    // Read PNG IHDR chunk: width at bytes 16-19, height at bytes 20-23 (big-endian uint32)
    const width = buffer.readUInt32BE(16);
    const height = buffer.readUInt32BE(20);
    expect(width).toBe(2160); // 1080 * deviceScaleFactor(2)
    expect(height).toBe(3840); // 1920 * deviceScaleFactor(2)
  });

  test('screenshot dimensions — square (1080×1080 at 2x = 2160×2160)', async ({
    request,
  }) => {
    test.skip(true, 'Requires running Docker stack with seeded data');
    const res = await request.get(
      '/api/screenshot/tracklist/test-id?format=square',
    );
    expect(res.status()).toBe(200);
    const body = await res.body();
    const buffer = Buffer.from(body);
    const width = buffer.readUInt32BE(16);
    const height = buffer.readUInt32BE(20);
    expect(width).toBe(2160);
    expect(height).toBe(2160);
  });

  test('unicode rendering — CJK track titles render without ? substitution', async ({
    page,
  }) => {
    test.skip(true, 'Requires running Docker stack with seeded data');
    // Navigate to the render page and check no "?" appears where CJK chars should be
    await page.goto('/render/tracklist/test-id');
    const cardText = await page.locator('#tracklist-card').innerText();
    // The CJK title from the test fixture should appear
    expect(cardText).toContain('夜桜お七');
    expect(cardText).not.toMatch(/\?{2,}/); // no sequences of ? (broken chars)
  });
});
