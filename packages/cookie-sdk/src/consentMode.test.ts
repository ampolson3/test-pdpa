import { afterEach, describe, expect, it } from 'vitest';
import { gtagDefault, gtagUpdate } from './consentMode';

type DataLayerEntry = unknown[];

function withWindow(): { dataLayer: DataLayerEntry[] } {
  const w = { dataLayer: [] as DataLayerEntry[] };
  (globalThis as unknown as { window: unknown }).window = w;
  return w;
}

afterEach(() => {
  delete (globalThis as unknown as { window?: unknown }).window;
});

// CON-03's own "รองรับ Google Consent Mode v2": pushes the fixed-vocabulary gtag consent signals onto
// window.dataLayer, which works whether or not gtag.js/GTM has actually loaded yet.
describe('gtagDefault', () => {
  it('pushes a safe default — everything denied except security_storage', () => {
    const w = withWindow();
    gtagDefault();
    expect(w.dataLayer).toEqual([
      [
        'consent',
        'default',
        {
          ad_storage: 'denied',
          ad_user_data: 'denied',
          ad_personalization: 'denied',
          analytics_storage: 'denied',
          functionality_storage: 'denied',
          personalization_storage: 'denied',
          security_storage: 'granted',
        },
      ],
    ]);
  });

  it('is a harmless no-op when window does not exist', () => {
    expect(() => gtagDefault()).not.toThrow();
  });
});

describe('gtagUpdate', () => {
  it('maps marketing to the three ad_* signals and analytics to analytics_storage', () => {
    const w = withWindow();
    gtagUpdate({ necessary: true, analytics: true, marketing: false });
    expect(w.dataLayer[0]).toEqual([
      'consent',
      'update',
      {
        ad_storage: 'denied',
        ad_user_data: 'denied',
        ad_personalization: 'denied',
        analytics_storage: 'granted',
        functionality_storage: 'denied',
        personalization_storage: 'denied',
        security_storage: 'granted',
      },
    ]);
  });

  it('maps functional to both functionality_storage and personalization_storage', () => {
    const w = withWindow();
    gtagUpdate({ functional: true });
    const signals = w.dataLayer[0][2] as Record<string, string>;
    expect(signals.functionality_storage).toBe('granted');
    expect(signals.personalization_storage).toBe('granted');
  });
});
