"use client";

import { useMemo } from "react";
import { useLocale, useTranslations } from "next-intl";
import { formatDate, type Locale } from "@pdpa/i18n";
import { createApiClient, useClauses, useDocumentTypes, type DocumentComparison } from "@pdpa/api-client";
import { Link } from "@/i18n/routing";

export const field = "rounded-md border border-slate-300 px-2 py-1.5";
export const input = `w-full ${field}`;
export const BFF = "/api/bff";

export function useClient() {
  return useMemo(() => createApiClient(BFF), []);
}

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

const tone: Record<string, string> = {
  draft: "bg-slate-100 text-slate-700", published: "bg-emerald-50 text-emerald-800", retired: "bg-slate-50 text-slate-500",
  pending: "bg-amber-50 text-amber-800", done: "bg-emerald-50 text-emerald-800", failed: "bg-rose-100 text-rose-900",
};

export function DocStatus({ status }: { status: string }) {
  const t = useTranslations("docs.status");
  return <span className={`whitespace-nowrap rounded-full px-2 py-0.5 text-xs ${tone[status] ?? tone.draft}`} data-status={status}>{t(status)}</span>;
}

/** Tabs between the composer's pages. */
export function DocsNav({ current }: { current: "documents" | "clauses" | "templates" }) {
  const t = useTranslations("docs.nav");
  const items = [
    { key: "documents", href: "/documents" },
    { key: "templates", href: "/documents/templates" },
    { key: "clauses", href: "/documents/clauses" },
  ] as const;
  return (
    <nav className="flex gap-4 border-b border-slate-200 text-sm">
      {items.map((i) => (
        <Link key={i.key} href={i.href} className={`-mb-px border-b-2 px-1 pb-2 ${current === i.key ? "border-slate-900 font-medium" : "border-transparent text-slate-600"}`}>
          {t(i.key)}
        </Link>
      ))}
    </nav>
  );
}

/** Merge field labels in the UI language and the published clauses, for the editor. */
export function useEditorCatalog(docType?: string) {
  const client = useClient();
  const locale = useLocale();
  const types = useDocumentTypes(client);
  const clauses = useClauses(client, { published_only: true, applies_to: docType as never });
  const fields = useMemo(
    () => (types.data?.fields ?? []).map((f) => ({ key: f.key, label: locale === "en" ? f.label.en : f.label.th })),
    [types.data, locale],
  );
  const clauseList = useMemo(
    () => (clauses.data ?? []).map((c) => ({ code: c.code, version: c.version, title: c.body.th.title })),
    [clauses.data],
  );
  return { types, fields, clauses: clauseList };
}

/** A version comparison: unchanged blocks dimmed, added / removed blocks marked, edits shown inside the text. */
export function ComparisonView({ comparison }: { comparison: DocumentComparison }) {
  const t = useTranslations("docs.editor");
  const s = comparison.summary;
  const changed = comparison.changes.some((c) => c.op !== "equal");
  return (
    <div className="space-y-2" data-testid="doc-comparison">
      <p className="text-slate-600">{t("summary", { insert: s.insert ?? 0, delete: s.delete ?? 0, change: s.change ?? 0 })}</p>
      {!changed && <p className="text-slate-500">{t("noChanges")}</p>}
      <ol className="space-y-1 rounded-md border border-slate-200 bg-white p-3">
        {comparison.changes.map((c, i) => (
          <li key={i} data-op={c.op} className={c.op === "equal" ? "text-slate-400" : c.op === "insert" ? "bg-emerald-50 text-emerald-900" : c.op === "delete" ? "bg-rose-50 text-rose-900 line-through" : ""}>
            <span className={c.kind.startsWith("heading") ? "font-semibold" : ""}>
              {c.op === "change"
                ? c.segments?.map((sg, j) =>
                    sg.op === "equal" ? <span key={j}>{sg.text}</span> : sg.op === "insert" ? <ins key={j} className="bg-emerald-100 no-underline">{sg.text}</ins> : <del key={j} className="bg-rose-100">{sg.text}</del>,
                  )
                : c.op === "delete" ? c.before : c.after}
            </span>
          </li>
        ))}
      </ol>
    </div>
  );
}
