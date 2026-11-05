// Vanilla TypeScript SDK for the cookie consent banner (CON-01) — built standalone (no React, no
// @pdpa/api-client) and embedded on tenant websites via a plain <script> tag, so its own bundle size is
// the acceptance criterion's own budget (≤ 30 KB gzip), not a Next.js app's. docs/modules/CON.md.
import { renderBanner } from './banner';
import { gtagDefault, gtagUpdate } from './consentMode';
import { applyPreferences, choicesForAction, guardDynamicScripts } from './scripts';
import { idempotencyKey, readConsent, visitorID, writeConsent } from './storage';
import type { ConsentAction, PublicBanner } from './types';

export interface InitOptions {
  /** The admin-issued public key naming this domain (platform.public_keys) */
  domainKey: string;
  /** Overrides the API origin — defaults to the script's own origin. */
  apiBase?: string;
}

function resolveApiBase(opts: InitOptions): string {
  if (opts.apiBase) return opts.apiBase.replace(/\/$/, '');
  const script = document.currentScript as HTMLScriptElement | null;
  if (script?.src) {
    try {
      return new URL(script.src).origin;
    } catch {
      /* fall through */
    }
  }
  return '';
}

async function fetchBanner(apiBase: string, domainKey: string): Promise<PublicBanner> {
  const res = await fetch(`${apiBase}/public/v1/cookie-banners/${encodeURIComponent(domainKey)}`, {
    headers: { 'Accept-Language': document.documentElement.lang?.toLowerCase().startsWith('en') ? 'en' : 'th' },
  });
  if (!res.ok) throw new Error(`pdpa-cookie-sdk: banner fetch failed (${res.status})`);
  return (await res.json()) as PublicBanner;
}

async function submitConsent(
  apiBase: string,
  domainKey: string,
  banner: PublicBanner,
  action: ConsentAction,
  choices: Record<string, boolean>,
): Promise<void> {
  const vid = visitorID();
  const res = await fetch(`${apiBase}/public/v1/cookie-banners/${encodeURIComponent(domainKey)}/consents`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': idempotencyKey() },
    body: JSON.stringify({
      domain_id: banner.domain_id,
      visitor_id: vid,
      banner_config_id: banner.id,
      action,
      choices,
    }),
  });
  if (!res.ok) throw new Error(`pdpa-cookie-sdk: consent submit failed (${res.status})`);
  writeConsent(domainKey, { version_no: banner.version_no, action, choices, decided_at: new Date().toISOString() });
}

export async function init(opts: InitOptions): Promise<void> {
  // CON-03 ("Prior blocking"): patches script insertion before anything else runs, synchronously, so a
  // tag manager injecting a sub-tag the instant *its own* container loads — which can happen before the
  // banner fetch below even resolves — is intercepted deterministically rather than raced against (no
  // MutationObserver: its callback is a microtask that runs too late to stop an already-started fetch,
  // which this package's own E2E test caught as a real failure during development). Every script it
  // intercepts before `guard.resolve` is first called is held, classified, and only then released or kept
  // inert — the literal "ก่อนยินยอมไม่มีคุกกี้ Analytics/Marketing ถูกตั้ง".
  const guard = guardDynamicScripts();
  gtagDefault();

  const apiBase = resolveApiBase(opts);
  let banner: PublicBanner;
  try {
    banner = await fetchBanner(apiBase, opts.domainKey);
  } catch (err) {
    console.error(err);
    return;
  }

  const existing = readConsent(opts.domainKey);
  if (existing && existing.version_no === banner.version_no) {
    // A returning visitor's own prior choice still takes effect on every page load (CON-02) — not just
    // the moment they decide it.
    applyPreferences(existing.choices, banner.script_rules);
    guard.resolve(existing.choices, banner.script_rules ?? []);
    gtagUpdate(existing.choices);
    return;
  }

  // No decision yet: default-deny every non-required category (CON-03's own acceptance criterion) until
  // the visitor actually decides — this is what "prior blocking" means for a first-time visitor, not just
  // "immediate effect" once they've clicked something (CON-02's own criterion).
  const denyAll = choicesForAction('reject_all', banner.categories);
  applyPreferences(denyAll, banner.script_rules);
  guard.resolve(denyAll, banner.script_rules ?? []);
  gtagUpdate(denyAll);

  const decide = (action: ConsentAction, choices: Record<string, boolean>) => {
    applyPreferences(choices, banner.script_rules); // immediate effect — CON-02's own acceptance criterion
    guard.resolve(choices, banner.script_rules ?? []);
    gtagUpdate(choices);
    void submitConsent(apiBase, opts.domainKey, banner, action, choices).then(() => el.remove());
  };

  const el = renderBanner(banner, {
    onAcceptAll: () => decide('accept_all', choicesForAction('accept_all', banner.categories)),
    onRejectAll: () => decide('reject_all', choicesForAction('reject_all', banner.categories)),
    onSaveCustom: (custom) => decide('custom', choicesForAction('custom', banner.categories, custom)),
  });
  // Only this DOM-insertion step needs document.body to exist — everything above it, including the
  // guard/gtagDefault calls at the top of this function, needs nothing but document.currentScript, so
  // init() itself is never deferred to DOMContentLoaded (CON-03's own fix: deferring the *whole* function
  // used to let a script placed later in the HTML install itself before the guard did — caught by this
  // package's own E2E test).
  if (document.body) document.body.appendChild(el);
  else document.addEventListener('DOMContentLoaded', () => document.body.appendChild(el));
}

// Auto-init from a <script src=".../pdpa-cookie-sdk.js" data-domain-key="..."> tag, the embed snippet shown
// on /settings/cookie-banners. A caller that wants explicit control can instead import { init } and skip
// this tag-driven path entirely (data-domain-key absent → nothing runs). Called unconditionally and
// synchronously, never deferred to DOMContentLoaded — see the comment at the top of init() for why.
(function autoInit(): void {
  if (typeof document === 'undefined') return;
  const script = document.currentScript as HTMLScriptElement | null;
  const key = script?.getAttribute('data-domain-key');
  if (!key) return;
  void init({ domainKey: key, apiBase: script?.getAttribute('data-api-base') ?? undefined });
})();
