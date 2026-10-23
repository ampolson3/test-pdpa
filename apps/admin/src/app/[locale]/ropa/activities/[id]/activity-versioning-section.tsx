"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import {
  type ApiClient,
  type ProcessingActivity,
  useActivityVersions,
  useCompareActivityVersions,
  useActivityMutations,
} from "@pdpa/api-client";

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

/** ROPA-13 (ST-05): approve/return a pending activity, reopen an approved one for review, and browse its
 *  approved version history — the acceptance criterion's "เก็บประวัติการแก้ไข" and "เปรียบเทียบเวอร์ชัน". */
export function ActivityVersioningSection({ client, activity }: { client: ApiClient; activity: ProcessingActivity }) {
  const t = useTranslations("activities");
  const locale = useLocale() as Locale;
  const canApprove = usePermission("ropa.activity.approve");
  const canUpdate = usePermission("ropa.activity.update");
  const m = useActivityMutations(client, activity.id);
  const versions = useActivityVersions(client, activity.id);
  const [returnReason, setReturnReason] = useState("");
  const [showReturn, setShowReturn] = useState(false);
  const [endReason, setEndReason] = useState("");
  const [showEnd, setShowEnd] = useState(false);
  const [fromId, setFromId] = useState("");
  const [toId, setToId] = useState("");
  const diff = useCompareActivityVersions(client, activity.id, fromId || undefined, toId || undefined);

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="versioning-section">
      <h2 className="font-semibold">{t("versioning.title")}</h2>

      {activity.status === "pending_approval" && canApprove && (
        <div className="space-y-2 rounded-md bg-sky-50 p-3">
          <p className="text-sky-900">{t("versioning.pendingApprovalHint")}</p>
          <div className="flex flex-wrap items-center gap-2">
            <Button onClick={() => m.approve.mutate(activity)} disabled={m.approve.isPending} data-testid="approve-activity">{t("versioning.approve")}</Button>
            <Button variant="secondary" onClick={() => setShowReturn((v) => !v)}>{t("versioning.return")}</Button>
          </div>
          {showReturn && (
            <div className="flex flex-wrap items-center gap-2">
              <input
                className="rounded-md border border-slate-300 px-2 py-1"
                placeholder={t("versioning.returnReason")}
                value={returnReason}
                onChange={(e) => setReturnReason(e.target.value)}
              />
              <Button
                variant="secondary"
                onClick={() => m.returnToDraft.mutate({ activity, reason: returnReason || undefined }, { onSuccess: () => { setShowReturn(false); setReturnReason(""); } })}
                disabled={m.returnToDraft.isPending}
                data-testid="return-activity"
              >{t("versioning.confirmReturn")}</Button>
            </div>
          )}
          {m.approve.isError && <p className="text-red-700" role="alert">{t("versioning.actionError", { detail: detail(m.approve.error) })}</p>}
          {m.returnToDraft.isError && <p className="text-red-700" role="alert">{t("versioning.actionError", { detail: detail(m.returnToDraft.error) })}</p>}
        </div>
      )}

      {activity.status === "active" && canUpdate && (
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <Button variant="secondary" onClick={() => m.requestReview.mutate(activity)} disabled={m.requestReview.isPending} data-testid="request-review">
              {t("versioning.requestReview")}
            </Button>
            <Button variant="secondary" onClick={() => setShowEnd((v) => !v)} data-testid="show-end-activity">
              {t("versioning.end")}
            </Button>
          </div>
          {m.requestReview.isError && <p className="text-red-700" role="alert">{t("versioning.actionError", { detail: detail(m.requestReview.error) })}</p>}
          {showEnd && (
            <div className="flex flex-wrap items-center gap-2 rounded-md bg-amber-50 p-3">
              <input
                className="rounded-md border border-slate-300 px-2 py-1"
                placeholder={t("versioning.endReason")}
                value={endReason}
                onChange={(e) => setEndReason(e.target.value)}
              />
              <Button
                variant="secondary"
                onClick={() =>
                  m.end.mutate({ activity, reason: endReason }, { onSuccess: () => { setShowEnd(false); setEndReason(""); } })
                }
                disabled={m.end.isPending || !endReason.trim()}
                data-testid="confirm-end-activity"
              >
                {t("versioning.confirmEnd")}
              </Button>
              {m.end.isError && <p className="text-red-700" role="alert">{t("versioning.actionError", { detail: detail(m.end.error) })}</p>}
            </div>
          )}
        </div>
      )}

      {activity.status === "under_review" && canUpdate && (
        <div className="flex items-center gap-2">
          <Button variant="secondary" onClick={() => m.confirmNoChange.mutate(activity)} disabled={m.confirmNoChange.isPending} data-testid="confirm-no-change">
            {t("versioning.confirmNoChange")}
          </Button>
          {m.confirmNoChange.isError && <p className="text-red-700" role="alert">{t("versioning.actionError", { detail: detail(m.confirmNoChange.error) })}</p>}
        </div>
      )}

      <div>
        <h3 className="font-medium">{t("versioning.history")}</h3>
        {(versions.data?.length ?? 0) === 0 ? (
          <p className="text-slate-500">{t("versioning.noVersions")}</p>
        ) : (
          <ul className="divide-y divide-slate-100" data-testid="version-list">
            {versions.data!.map((v) => (
              <li key={v.id} className="flex items-center justify-between py-2">
                <span>
                  {t("versioning.versionNo", { no: v.version_no })}
                  {" — "}
                  {v.approved_at ? formatDate(v.approved_at, locale, { day: "numeric", month: "short", year: "numeric" }) : "—"}
                </span>
                <span className="flex gap-2 text-xs">
                  <button className="text-sky-700 underline" onClick={() => setFromId(v.id)}>{t("versioning.compareFrom")}</button>
                  <button className="text-sky-700 underline" onClick={() => setToId(v.id)}>{t("versioning.compareTo")}</button>
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>

      {fromId && toId && (
        <div className="rounded-md bg-slate-50 p-3" data-testid="version-diff">
          <h3 className="font-medium">{t("versioning.diffTitle")}</h3>
          {(diff.data?.length ?? 0) === 0 ? (
            <p className="text-slate-500">{t("versioning.noDiff")}</p>
          ) : (
            <ul className="space-y-1">
              {diff.data!.map((d) => (
                <li key={d.section}>
                  <span className="font-medium">{d.section}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </section>
  );
}
