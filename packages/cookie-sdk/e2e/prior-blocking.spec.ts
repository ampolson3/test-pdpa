import { expect, test } from '@playwright/test';

const BANNER = {
  id: 'bc-1',
  domain_id: 'dom-1',
  version_no: 1,
  layout: 'bar',
  texts: {
    title_th: 'เราใช้คุกกี้',
    accept_all_th: 'ยอมรับทั้งหมด',
    reject_all_th: 'ปฏิเสธทั้งหมด',
    settings_th: 'ตั้งค่า',
  },
  categories: [
    { id: 'c1', code: 'necessary', name_th: 'จำเป็น', is_required: true, display_order: 0 },
    { id: 'c2', code: 'analytics', name_th: 'วิเคราะห์', is_required: false, display_order: 1 },
    { id: 'c3', code: 'marketing', name_th: 'การตลาด', is_required: false, display_order: 2 },
  ],
  script_rules: [{ pattern: 'marketing-tag.js', category_code: 'marketing', action: 'block' }],
};

test.beforeEach(async ({ page }) => {
  await page.route('**/public/v1/cookie-banners/*/consents', (route) =>
    route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ recorded: true }) }),
  );
  await page.route('**/public/v1/cookie-banners/*', (route) => {
    if (route.request().url().includes('/consents')) return route.fallback();
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(BANNER) });
  });
});

// CON-03's own literal acceptance criterion: before any consent decision, no Analytics/Marketing cookie
// is ever set — neither the declaratively-gated analytics tag nor the plain third-party marketing <script
// src> the page embeds (mirroring a real site that didn't bother gating it declaratively).
test('no analytics or marketing cookie is set before any decision', async ({ page }) => {
  await page.goto('/');
  await page.waitForTimeout(500); // give the SDK's own prior-blocking sweep a moment to run

  const cookies = await page.context().cookies();
  expect(cookies.find((c) => c.name === '_fake_ga')).toBeUndefined();
  expect(cookies.find((c) => c.name === '_fake_fbp')).toBeUndefined();
});

// CON-02's own acceptance criterion, still true on this same example site: accepting activates the
// declaratively-gated analytics script immediately.
test('accepting all activates the gated analytics script immediately', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'ยอมรับทั้งหมด' }).click();
  await page.waitForFunction(() => document.cookie.includes('_fake_ga='));

  const cookies = await page.context().cookies();
  expect(cookies.find((c) => c.name === '_fake_ga')).toBeDefined();
});

// Rejecting leaves both disallowed categories' scripts untouched (still never set).
test('rejecting all keeps both disallowed scripts blocked', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'ปฏิเสธทั้งหมด' }).click();
  await page.waitForTimeout(200);

  const cookies = await page.context().cookies();
  expect(cookies.find((c) => c.name === '_fake_ga')).toBeUndefined();
  expect(cookies.find((c) => c.name === '_fake_fbp')).toBeUndefined();
});
