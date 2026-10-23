// CON-03's own "รองรับ Google Consent Mode v2" — this SDK has no dependency on gtag.js/GTM actually being
// present: Google Consent Mode works by pushing fixed-shape entries onto `window.dataLayer` (the same
// array gtag.js and GTM both read once *they* load), so these calls are safe and inert on a page that
// never loads either. Per https://developers.google.com/tag-platform/security/guides/consent, the eight
// granular signals are fixed vocabulary — only `ad_storage`/`ad_user_data`/`ad_personalization` (marketing)
// and `analytics_storage` (analytics) are driven by a *tenant's own* category codes; `functionality_storage`/
// `personalization_storage` follow the `functional` category when present, and `security_storage` is always
// granted (it's not something a consent banner can refuse — it's what keeps the site itself working).
type ConsentState = 'granted' | 'denied';
type ConsentSignals = Record<string, ConsentState>;

function push(...args: unknown[]): void {
  if (typeof window === 'undefined') return;
  const w = window as unknown as { dataLayer?: unknown[] };
  w.dataLayer = w.dataLayer ?? [];
  w.dataLayer.push(args);
}

function signalsForChoices(choices: Record<string, boolean>): ConsentSignals {
  const granted = (code: string) => (choices[code] ? 'granted' : 'denied');
  return {
    ad_storage: granted('marketing'),
    ad_user_data: granted('marketing'),
    ad_personalization: granted('marketing'),
    analytics_storage: granted('analytics'),
    functionality_storage: granted('functional'),
    personalization_storage: granted('functional'),
    security_storage: 'granted',
  };
}

/** Called as early as possible on every page load, before any tag manager script runs, so GTM/gtag.js
 * picks up the safe default (everything but security denied) the moment it initializes — the literal point
 * of "prior blocking" for Consent Mode. */
export function gtagDefault(): void {
  push('consent', 'default', signalsForChoices({}));
}

/** Called the moment a real decision (or a returning visitor's stored one) is known. */
export function gtagUpdate(choices: Record<string, boolean>): void {
  push('consent', 'update', signalsForChoices(choices));
}
