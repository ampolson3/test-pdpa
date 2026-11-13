import { defineConfig } from '@playwright/test';

// CON-03's own literal acceptance criterion: "ก่อนยินยอมไม่มีคุกกี้ Analytics/Marketing ถูกตั้ง (Playwright
// test บนเว็บตัวอย่าง)" — a Playwright test against an example website. e2e/fixtures/index.html is that
// website; e2e/server.mjs serves it (and the real built SDK bundle) statically, with no backend — the test
// itself mocks the /public/v1 calls this SDK makes.
export default defineConfig({
  testDir: './e2e',
  timeout: 15_000,
  webServer: {
    command: 'node e2e/server.mjs',
    port: 4310,
    reuseExistingServer: !process.env.CI,
  },
  use: {
    baseURL: 'http://localhost:4310',
    // This environment's pre-installed Chromium revision doesn't match the one @playwright/test 1.51.1
    // would otherwise try to download — point at it directly instead of fetching a second copy.
    launchOptions: { executablePath: '/opt/pw-browsers/chromium' },
  },
});
