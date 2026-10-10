"use client";

import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { createApiClient, useActivityRiskScore, useScoreActivity } from "@pdpa/api-client";

const LEVEL_STYLE: Record<string, string> = {
  low: "bg-emerald-100 text-emerald-800",
  medium: "bg-amber-100 text-amber-800",
  high: "bg-orange-100 text-orange-800",
  very_high: "bg-red-100 text-red-800",
};

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

/** RRA-01: the activity's risk score, computed live from its own RoPA data against the tenant's default
 *  matrix (RRA-02) — a fresh score any time "คำนวณใหม่" is pressed, with the factors that moved it listed
 *  for explainability (the acceptance criterion's own "อธิบายปัจจัยที่ทำให้สูงได้"). */
export function RiskScoreSection({ client, activityId }: { client: ReturnType<typeof createApiClient>; activityId: string }) {
  const t = useTranslations("activities");
  const canRead = usePermission("ropa.risk.read");
  const canScore = usePermission("ropa.risk.create");
  const latest = useActivityRiskScore(client, activityId);
  const score = useScoreActivity(client);

  if (!canRead) return null;

  const current = score.data ?? latest.data;

  return (
    <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="risk-score-section">
      <div className="flex items-center justify-between">
        <h2 className="font-semibold">{t("sections.riskScore")}</h2>
        {canScore && <Button variant="secondary" onClick={() => score.mutate(activityId)} disabled={score.isPending}>{t("riskScore.recompute")}</Button>}
      </div>
      {score.isError && <p className="text-red-700" role="alert">{t("riskScore.scoreError", { detail: detail(score.error) })}</p>}
      {!current && latest.isError && !score.isPending ? (
        <p className="text-slate-500">{t("riskScore.notScoredYet")}</p>
      ) : !current ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : (
        <div className="space-y-2" data-testid="risk-score-result">
          <div className="flex items-center gap-2">
            <span className={`rounded px-2 py-1 text-xs font-semibold ${LEVEL_STYLE[current.level] ?? "bg-slate-100 text-slate-700"}`}>
              {t(`riskScore.levels.${current.level}`)}
            </span>
            <span className="text-slate-600">{t("riskScore.score", { score: current.score, likelihood: current.likelihood, impact: current.impact })}</span>
          </div>
          {(current.level === "high" || current.level === "very_high") && (
            <p className="rounded bg-orange-50 px-3 py-2 text-sm text-orange-900" role="status">
              {t("riskScore.dpiaTriggered")}
            </p>
          )}
          {current.factors.length === 0 ? (
            <p className="text-slate-500">{t("riskScore.noFactors")}</p>
          ) : (
            <ul className="list-inside list-disc text-slate-700">
              {current.factors.map((f, i) => (
                <li key={i}>{t(`riskScore.factors.${f.code}`)} ({t(`riskScore.contributesTo.${f.contributes_to}`)})</li>
              ))}
            </ul>
          )}
        </div>
      )}
    </section>
  );
}
