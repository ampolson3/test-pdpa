"use client";

import { useLocale, useTranslations } from "next-intl";
import { formatDate, type Locale } from "@pdpa/i18n";
import type { BreachIncident } from "@pdpa/api-client";

export const field = "rounded-md border border-slate-300 px-2 py-1.5";
export const input = `w-full ${field}`;

export function problemText(e: unknown): string {
  if (typeof e !== "object" || e === null) return "";
  const p = e as { title?: string; detail?: string; errors?: { field: string; code: string }[] };
  const fields = (p.errors ?? []).map((f) => `${f.field}: ${f.code}`).join(", ");
  return [p.title, p.detail, fields].filter(Boolean).join(" — ");
}

export function useWhen() {
  const locale = useLocale() as Locale;
  return (d?: string | null) => (d ? formatDate(d, locale, { month: "short", hour: "2-digit", minute: "2-digit" }) : "");
}

const statusTone: Record<string, string> = {
  reported: "bg-rose-50 text-rose-800", triage: "bg-amber-50 text-amber-800", assessing: "bg-amber-50 text-amber-800",
  notifying: "bg-sky-50 text-sky-800", remediating: "bg-slate-100 text-slate-700", closed: "bg-emerald-50 text-emerald-800",
};

export function StatusBadge({ status }: { status: string }) {
  const t = useTranslations("breach.status");
  return <span className={`whitespace-nowrap rounded-full px-2 py-0.5 text-xs ${statusTone[status] ?? statusTone.remediating}`} data-status={status}>{t(status)}</span>;
}

const riskTone: Record<string, string> = { none: "bg-slate-100 text-slate-700", low: "bg-amber-50 text-amber-800", high: "bg-rose-100 text-rose-900" };

export function RiskBadge({ risk }: { risk?: string }) {
  const t = useTranslations("breach.risk");
  if (!risk) return null;
  return <span className={`whitespace-nowrap rounded-full px-2 py-0.5 text-xs ${riskTone[risk]}`} data-risk={risk}>{t(risk)}</span>;
}

const clockTone: Record<string, string> = {
  on_track: "bg-emerald-50 text-emerald-800", due_soon: "bg-amber-100 text-amber-900", overdue: "bg-rose-600 text-white", stopped: "bg-slate-100 text-slate-600",
};

/** The 72-hour clock (BRE-07): hours left to the PDPC notice, or overdue. */
export function Clock({ incident }: { incident: BreachIncident }) {
  const t = useTranslations("breach.clock");
  const c = incident.clock;
  const left = Math.max(0, 72 - c.hours_elapsed);
  const label =
    c.state === "stopped" ? t("stopped") : c.state === "overdue" ? t("overdue", { hours: Math.floor(c.hours_elapsed - 72) }) : t("left", { hours: Math.floor(left), minutes: Math.floor((left % 1) * 60) });
  return <span className={`whitespace-nowrap rounded-md px-2 py-0.5 text-xs font-medium ${clockTone[c.state]}`} data-clock={c.state} data-testid="clock">{label}</span>;
}
