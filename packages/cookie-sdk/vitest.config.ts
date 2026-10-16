import { defineConfig } from 'vitest/config';

// Scope vitest to src/*.test.ts only — e2e/*.spec.ts are Playwright tests (playwright.config.ts), run via
// `pnpm test:e2e`, and must never be picked up by vitest's own default glob.
export default defineConfig({
  test: {
    include: ['src/**/*.test.ts'],
  },
});
