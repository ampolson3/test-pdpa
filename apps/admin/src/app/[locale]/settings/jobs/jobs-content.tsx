"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import { createApiClient, useJobs, type JobState } from "@pdpa/api-client";

const STATES: JobState[] = ["running", "available", "scheduled", "retryable", "pending", "completed", "discarded", "cancelled"];

const stateStyle: Record<JobState, string> = {
  running: "bg-blue-100 text-blue-800",
  available: "bg-slate-100 text-slate-700",
  scheduled: "bg-slate-100 text-slate-700",
  pending: "bg-slate-100 text-slate-700",
  retryable: "bg-amber-100 text-amber-800",
  completed: "bg-green-100 text-green-800",
  discarded: "bg-red-100 text-red-800",
  cancelled: "bg-red-100 text-red-800",
};

const timeFormat: Intl.DateTimeFormatOptions = { month: "short", hour: "2-digit", minute: "2-digit", second: "2-digit" };

export function JobsContent() {
  const t = useTranslations("jobs");
  const allowed = usePermission("admin.job.read");

  if (!allowed) {
    return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;
  }
  return (
    <main className="mx-auto max-w-5xl space-y-4 p-8">
      <header>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-sm text-slate-600">{t("description")}</p>
      </header>
      <JobsTable />
    </main>
  );
}

function JobsTable() {
  const t = useTranslations("jobs");
  const locale = useLocale() as Locale;
  // Client components go through this app's BFF, which attaches the token server-side.
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [state, setState] = useState<JobState | "">("");
  const [kind, setKind] = useState("");

  const query = useJobs(client, { state: state ? [state] : undefined, kind: kind.trim() || undefined });
  const jobs = query.data?.pages.flatMap((p) => p.data) ?? [];

  return (
    <>
      <div className="flex flex-wrap items-end gap-3">
        <label className="text-sm">
          <span className="block text-slate-600">{t("filter.state")}</span>
          <select
            className="mt-1 rounded-md border border-slate-300 bg-white px-2 py-1"
            value={state}
            onChange={(e) => setState(e.target.value as JobState | "")}
          >
            <option value="">{t("filter.allStates")}</option>
            {STATES.map((s) => (
              <option key={s} value={s}>
                {t(`state.${s}`)}
              </option>
            ))}
          </select>
        </label>
        <label className="text-sm">
          <span className="block text-slate-600">{t("filter.kind")}</span>
          <input
            className="mt-1 rounded-md border border-slate-300 bg-white px-2 py-1"
            value={kind}
            maxLength={100}
            placeholder="outbox.dispatch"
            onChange={(e) => setKind(e.target.value)}
          />
        </label>
      </div>

      {query.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : query.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : jobs.length === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : (
        <div className="overflow-x-auto rounded-md border border-slate-200 bg-white">
          <table className="w-full text-left text-sm">
            <thead className="bg-slate-50 text-slate-600">
              <tr>
                <th className="px-3 py-2 font-medium">{t("column.kind")}</th>
                <th className="px-3 py-2 font-medium">{t("column.state")}</th>
                <th className="px-3 py-2 font-medium">{t("column.attempts")}</th>
                <th className="px-3 py-2 font-medium">{t("column.createdAt")}</th>
                <th className="px-3 py-2 font-medium">{t("column.nextRunOrFinished")}</th>
                <th className="px-3 py-2 font-medium">{t("column.lastError")}</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((job) => (
                <tr key={job.id} className="border-t border-slate-100 align-top">
                  <td className="px-3 py-2 font-mono text-xs">{job.kind}</td>
                  <td className="px-3 py-2">
                    <span className={`rounded-full px-2 py-0.5 text-xs ${stateStyle[job.state]}`}>{t(`state.${job.state}`)}</span>
                  </td>
                  <td className="px-3 py-2 tabular-nums">
                    {job.attempt} / {job.max_attempts}
                  </td>
                  <td className="px-3 py-2 whitespace-nowrap">{formatDate(job.created_at, locale, timeFormat)}</td>
                  <td className="px-3 py-2 whitespace-nowrap">
                    {formatDate(job.finalized_at ?? job.scheduled_at, locale, timeFormat)}
                  </td>
                  <td className="max-w-xs truncate px-3 py-2 text-slate-600" title={job.last_error ?? undefined}>
                    {job.last_error ?? "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {query.hasNextPage && (
        <Button variant="secondary" onClick={() => query.fetchNextPage()} disabled={query.isFetchingNextPage}>
          {t("loadMore")}
        </Button>
      )}
    </>
  );
}
