import type { StoredConsent } from './types';

const VISITOR_COOKIE = 'pdpa_cvid';
const CONSENT_COOKIE_PREFIX = 'pdpa_consent_'; // one cookie per domain key, so two tenants on one browser never collide
const CONSENT_MAX_AGE_DAYS = 180;

function readCookie(name: string): string | null {
  const row = document.cookie.split('; ').find((c) => c.startsWith(name + '='));
  return row ? decodeURIComponent(row.slice(name.length + 1)) : null;
}

function writeCookie(name: string, value: string, maxAgeDays: number): void {
  const maxAge = maxAgeDays * 24 * 60 * 60;
  document.cookie = `${name}=${encodeURIComponent(value)}; max-age=${maxAge}; path=/; SameSite=Lax`;
}

function randomID(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
  return 'v' + Math.random().toString(36).slice(2) + Date.now().toString(36);
}

// visitorID is the SDK's own first-party cookie value (RecordConsentInput.VisitorID) — stable across
// visits so repeat consent events from the same browser are traceable, never a cross-site identifier.
export function visitorID(): string {
  let id = readCookie(VISITOR_COOKIE);
  if (!id) {
    id = randomID();
    writeCookie(VISITOR_COOKIE, id, 365);
  }
  return id;
}

export function readConsent(domainKey: string): StoredConsent | null {
  const raw = readCookie(CONSENT_COOKIE_PREFIX + domainKey);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as StoredConsent;
  } catch {
    return null;
  }
}

export function writeConsent(domainKey: string, consent: StoredConsent): void {
  writeCookie(CONSENT_COOKIE_PREFIX + domainKey, JSON.stringify(consent), CONSENT_MAX_AGE_DAYS);
}

export function idempotencyKey(): string {
  return randomID();
}
