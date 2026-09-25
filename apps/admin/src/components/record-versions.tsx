"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import { createApiClient, useCompareVersions, useRecordVersions, useVersionMutations, type RecordVersion } from "@pdpa/api-client";
import { VersionDiff } from "./version-diff";

const tone: Record<RecordVersion["status"], string> = {
  draft: "bg-slate-100 text-slate-700",
  in_review: "bg-amber-50 text-amber-800",
  approved: "bg-sky-50 text-sky-800",
  published: "bg-emerald-50 text-emerald-800",
  superseded: "bg-slate-50 text-slate-500",
};

export function VersionStatus({ status }: { status: RecordVersion["status"] }) {
  const t = useTranslations("versions.status");
  return <span className={`rounded-full px-2 py-0.5 text-xs ${tone[status]}`} data-version-status={status}>{t(status)}</span>;
}

/**
 * The version bar and history of a record (PLT-08): the published version, each version's status and
 * approval steps, submit / publish where allowed (the server decides), and a comparison of any two.
 * Modules mount it next to their editor; saving drafts stays in the module's own form.
 */
export function RecordVersions({ entityType, entityId, canEdit, canPublish }: { entityType: string; entityId: string; canEdit: boolean; canPublish: boolean }) {
  const t = useTranslations("versions");
  const locale = useLocale() as Locale;
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const list = useRecordVersions(client, entityType, entityId);
  const m = useVersionMutations(client);
  const [compare, setCompare] = useState<{ from?: string; to?: string }>({});
  const diff = useCompareVersions(client, compare.from, compare.to);

  if (list.isPending) return <p className="text-slate-500">{t("loading")}</p>;
  if (list.isError) return <p className="text-red-700">{t("loadError")}</p>;
  const published = list.data.find((v) => v.status === "published");
  const busy = m.submit.isPending || m.publish.isPending;
  const failed = m.submit.isError || m.publish.isError;

  return (
    <section className="space-y-3 text-sm" data-testid="record-versions">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-semibold">{t("title")}</h2>
        <span className="text-slate-600">{published ? t("current", { no: published.version }) : t("none")}</span>
      </header>
      <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
        {list.data.map((v) => (
          <li key={v.id} className="space-y-1 px-3 py-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="flex items-center gap-2">
                <span className="font-medium">{t("version", { no: v.version })}</span>
                <VersionStatus status={v.status} />
                <span className="text-xs text-slate-500">{v.author_name && t("by", { name: v.author_name })} · {formatDate(v.updated_at, locale, { month: "short", hour: "2-digit", minute: "2-digit" })}</span>
              </span>
              <span className="flex gap-2">
                {canEdit && v.status === "draft" && <Button onClick={() => m.submit.mutate(v)} disabled={busy}>{t("submit")}</Button>}
                {canPublish && v.status === "approved" && <Button onClick={() => m.publish.mutate(v)} disabled={busy}>{t("publish")}</Button>}
                {list.data.length > 1 && (
                  <Button variant="secondary" onClick={() => setCompare({ from: list.data.find((x) => x.id !== v.id && x.version < v.version)?.id ?? list.data.find((x) => x.id !== v.id)?.id, to: v.id })}>{t("compare")}</Button>
                )}
              </span>
            </div>
            {v.approvals.length > 0 && (
              <ol className="flex flex-wrap gap-2 text-xs text-slate-600">
                {v.approvals.map((a) => (
                  <li key={a.id} className="rounded bg-slate-50 px-2 py-0.5">
                    {t("step", { step: a.step, role: a.role })}: {t(`decision.${a.decision}`)}{a.approver_name ? ` · ${a.approver_name}` : ""}{a.reason ? ` — ${a.reason}` : ""}
                  </li>
                ))}
              </ol>
            )}
          </li>
        ))}
      </ul>
      {failed && <p className="text-red-700" role="alert">{t("actionError")}</p>}
      {compare.to && (
        <div className="space-y-2">
          <label className="flex items-center gap-2">
            {t("compareWith")}
            <select className="rounded-md border border-slate-300 bg-white px-2 py-1" value={compare.from ?? ""} onChange={(e) => setCompare({ ...compare, from: e.target.value })}>
              {list.data.filter((x) => x.id !== compare.to).map((x) => <option key={x.id} value={x.id}>{t("version", { no: x.version })}</option>)}
            </select>
          </label>
          {diff.data && <VersionDiff changes={diff.data} />}
        </div>
      )}
    </section>
  );
}
