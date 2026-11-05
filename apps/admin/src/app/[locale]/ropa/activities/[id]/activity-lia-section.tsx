"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { type ApiClient, useAssessLia, useDecideLia, useLiaForActivity } from "@pdpa/api-client";

const LIA_KEYS = ["purpose_legitimate", "necessary_for_purpose", "not_overridden_by_rights", "safeguards_in_place"] as const;

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

/** DPIA-16 (ม.24(5)): the legitimate-interest assessment (LIA) — answer the 3-part test (purpose /
 *  necessity / balancing) against this activity, and the DPO approves or rejects the round. A purpose
 *  relying on this basis isn't complete (ROPA-03's own completeness check) until a round here is approved. */
export function ActivityLiaSection({ client, activityId }: { client: ApiClient; activityId: string }) {
  const t = useTranslations("dpia");
  const canAssess = usePermission("assessment.dpia.update");
  const canApprove = usePermission("assessment.dpia.approve");
  const canRead = usePermission("assessment.dpia.read");
  const rounds = useLiaForActivity(client, activityId);
  const assess = useAssessLia(client);
  const decide = useDecideLia(client);
  const [answers, setAnswers] = useState<Record<string, "yes" | "no">>(
    Object.fromEntries(LIA_KEYS.map((k) => [k, "no"])) as Record<string, "yes" | "no">,
  );
  const [reason, setReason] = useState("");

  if (!canRead) return null;
  const list = rounds.data?.data ?? [];

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="activity-lia-section">
      <h2 className="font-semibold">{t("lia.title")}</h2>

      {canAssess && (
        <div className="space-y-2" data-testid="lia-form">
          <div className="grid gap-2 sm:grid-cols-2">
            {LIA_KEYS.map((key) => (
              <label key={key} className="flex items-start gap-2 text-sm">
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={answers[key] === "yes"}
                  onChange={(e) => setAnswers({ ...answers, [key]: e.target.checked ? "yes" : "no" })}
                />
                <span>{t(`lia.questions.${key}`)}</span>
              </label>
            ))}
          </div>
          {assess.isError && (
            <p className="text-red-700" role="alert">
              {t("submitError", { detail: detail(assess.error) })}
            </p>
          )}
          <Button onClick={() => assess.mutate({ activityId, answers })} disabled={assess.isPending} data-testid="lia-submit">
            {t("lia.submit")}
          </Button>
        </div>
      )}

      {rounds.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : list.length === 0 ? (
        <p className="rounded-md bg-slate-50 p-3 text-slate-600">{t("lia.empty")}</p>
      ) : (
        <ul className="space-y-2" data-testid="lia-rounds">
          {list.map((r) => (
            <li key={r.id} className="flex items-center justify-between gap-2 rounded border border-slate-100 p-2 text-sm">
              <span>
                {t("lia.round", { round: r.round_no })} —{" "}
                <span className={r.status === "approved" ? "text-green-700" : r.status === "rejected" ? "text-red-700" : "text-amber-700"}>
                  {t(`lia.statuses.${r.status}`)}
                </span>
              </span>
              {canApprove && r.status === "in_progress" && (
                <span className="flex items-center gap-1">
                  <input
                    className="w-32 rounded border border-slate-300 px-1 py-0.5 text-xs"
                    placeholder={t("lia.reasonPlaceholder")}
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                  />
                  <Button
                    onClick={() => decide.mutate({ activityId, liaId: r.id, ifMatch: `"${r.row_version}"`, to: "approved" })}
                    disabled={decide.isPending}
                  >
                    {t("lia.approve")}
                  </Button>
                  <Button
                    onClick={() => decide.mutate({ activityId, liaId: r.id, ifMatch: `"${r.row_version}"`, to: "rejected", reason })}
                    disabled={decide.isPending}
                  >
                    {t("lia.reject")}
                  </Button>
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
      {decide.isError && (
        <p className="text-red-700" role="alert">
          {t("submitError", { detail: detail(decide.error) })}
        </p>
      )}
    </section>
  );
}
