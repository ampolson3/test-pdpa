import type { Locale } from "./index";

const TIME_ZONE = "Asia/Bangkok";

/**
 * Formats an RFC 3339 UTC timestamp (api/openapi/README.md's Payload convention) in Asia/Bangkok,
 * Buddhist Era for th / Gregorian for en (CLAUDE.md rule 11: "Buddhist-era years only in the UI
 * layer via the locale"). `options` extends the default date-only format, e.g. to add a time.
 */
export function formatDate(iso: string, locale: Locale, options?: Intl.DateTimeFormatOptions): string {
  const date = new Date(iso);
  return new Intl.DateTimeFormat(intlLocale(locale), {
    timeZone: TIME_ZONE,
    year: "numeric",
    month: "long",
    day: "numeric",
    ...options,
  }).format(date);
}

function intlLocale(locale: Locale): string {
  return locale === "th" ? "th-TH-u-ca-buddhist" : "en-US";
}
