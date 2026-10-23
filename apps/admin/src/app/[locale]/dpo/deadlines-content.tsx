"use client";

import { useMemo } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/routing";
import { usePermission } from "@pdpa/authz";
import { formatDate, type Locale } from "@pdpa/i18n";
import { createApiClient, useDeadlines, type DpoDeadlineItem } from "@pdpa/api-client";

const statusStyle: Record<string, string> = {
  at_risk: "bg-amber-100 text-amber-800",
  overdue: "bg-red-100 text-red-800",
};

const dateFormat: Intl.DateTimeFormatOptions = { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" };

function hrefFor(item: DpoDeadlineItem): string {
  return item.source === "breach_incident" ? `/incidents/${item.reference_id}` : "/requests";
}

/** DPO-05: near-deadline and overdue work from every module that already tracks a legal deadline, on one page. */
export function DeadlinesContent() {
  const t = useTranslations("dpoDeadlines");
  const locale = useLocale() as Locale;
  const allowed = usePermission("dpo.report.read");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const query = useDeadlines(client);

  if (!allowed) {
    return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;
  }
  const items = query.data?.data ?? [];

  return (
    <main className="mx-auto max-w-5xl space-y-4 p-8">
      <header>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-sm text-slate-600">{t("description")}</p>
      </header>

      {query.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : query.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : items.length === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : (
        <div className="overflow-x-auto rounded-md border border-slate-200 bg-white" data-testid="deadlines-table">
          <table className="w-full text-left text-sm">
            <thead className="bg-slate-50 text-slate-600">
              <tr>
                <th className="px-3 py-2 font-medium">{t("column.source")}</th>
                <th className="px-3 py-2 font-medium">{t("column.reference")}</th>
                <th className="px-3 py-2 font-medium">{t("column.dueAt")}</th>
                <th className="px-3 py-2 font-medium">{t("column.status")}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={`${item.source}:${item.reference_id}`} className="border-t border-slate-100">
                  <td className="px-3 py-2">{t(`source.${item.source}`)}</td>
                  <td className="px-3 py-2">
                    <Link className="text-sky-700 underline" href={hrefFor(item)}>
                      {item.reference_label}
                    </Link>
                  </td>
                  <td className="px-3 py-2 whitespace-nowrap">{formatDate(item.due_at, locale, dateFormat)}</td>
                  <td className="px-3 py-2">
                    <span className={`rounded-full px-2 py-0.5 text-xs ${statusStyle[item.status]}`}>{t(`status.${item.status}`)}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}
