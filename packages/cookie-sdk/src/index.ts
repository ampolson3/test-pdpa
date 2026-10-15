// Vanilla TypeScript SDK for the cookie consent banner (CON-01) — built standalone (no React, no
// @pdpa/api-client) and embedded on tenant websites via a plain <script> tag, so its own bundle size is
// the acceptance criterion's own budget (≤ 30 KB gzip), not a Next.js app's. docs/modules/CON.md.
import { renderBanner } from './banner';
import { applyPreferences, choicesForAction } from './scripts';
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
    return;
  }

  const decide = (action: ConsentAction, choices: Record<string, boolean>) => {
    applyPreferences(choices, banner.script_rules); // immediate effect — the acceptance criterion itself
    void submitConsent(apiBase, opts.domainKey, banner, action, choices).then(() => el.remove());
  };

  const el = renderBanner(banner, {
    onAcceptAll: () => decide('accept_all', choicesForAction('accept_all', banner.categories)),
    onRejectAll: () => decide('reject_all', choicesForAction('reject_all', banner.categories)),
    onSaveCustom: (custom) => decide('custom', choicesForAction('custom', banner.categories, custom)),
  });
  document.body.appendChild(el);
}

// Auto-init from a <script src=".../pdpa-cookie-sdk.js" data-domain-key="..."> tag, the embed snippet shown
// on /settings/cookie-banners. A caller that wants explicit control can instead import { init } and skip
// this tag-driven path entirely (data-domain-key absent → nothing runs).
(function autoInit(): void {
  if (typeof document === 'undefined') return;
  const script = document.currentScript as HTMLScriptElement | null;
  const key = script?.getAttribute('data-domain-key');
  if (!key) return;
  const run = () => void init({ domainKey: key, apiBase: script?.getAttribute('data-api-base') ?? undefined });
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', run);
  } else {
    run();
  }
})();
