"use client";

import Link from "next/link";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { createApiClient, useActivityGapFindings, useAnalyzeActivity, useGapRules } from "@pdpa/api-client";

const SEVERITY_STYLE: Record<string, string> = {
  low: "bg-slate-100 text-slate-700",
  medium: "bg-amber-100 text-amber-800",
  high: "bg-red-100 text-red-800",
};

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

/** RRA-04: this activity's own legal-gap findings, as last analyzed, with a button to re-run every
 *  active rule against its current RoPA data now (the acceptance criterion — every rule type must be
 *  detected). A gap that no longer applies is cleared automatically on the next analysis. */
export function GapAnalysisSection({ client, activityId }: { client: ReturnType<typeof createApiClient>; activityId: string }) {
  const t = useTranslations("activities");
  const canRead = usePermission("ropa.risk.read");
  const canAnalyze = usePermission("ropa.risk.create");
  const findings = useActivityGapFindings(client, activityId);
  const rules = useGapRules(client);
  const analyze = useAnalyzeActivity(client);

  if (!canRead) return null;

  const severityByCode = new Map((rules.data ?? []).map((r) => [r.code, r.severity]));
  const open = (findings.data ?? []).filter((f) => f.status === "open");

  return (
    <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="gap-analysis-section">
      <div className="flex items-center justify-between">
        <h2 className="font-semibold">{t("sections.gaps")}</h2>
        <div className="flex items-center gap-2">
          <Link className="text-sky-700 underline" href="/settings/gap-register">{t("gaps.viewRegister")}</Link>
          {canAnalyze && <Button variant="secondary" onClick={() => analyze.mutate(activityId)} disabled={analyze.isPending}>{t("gaps.analyze")}</Button>}
        </div>
      </div>
      {analyze.isError && <p className="text-red-700" role="alert">{t("gaps.analyzeError", { detail: detail(analyze.error) })}</p>}
      {findings.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : !findings.data || findings.data.length === 0 ? (
        <p className="text-slate-500">{t("gaps.notAnalyzedYet")}</p>
      ) : open.length === 0 ? (
        <p className="text-emerald-700">{t("gaps.none")}</p>
      ) : (
        <ul className="divide-y divide-slate-100" data-testid="gap-findings-list">
          {open.map((f) => (
            <li key={f.id} className="flex items-center gap-2 py-2">
              <span className={`rounded px-2 py-0.5 text-xs font-semibold ${SEVERITY_STYLE[severityByCode.get(f.rule_code) ?? ""] ?? "bg-slate-100 text-slate-700"}`}>
                {t(`gaps.rules.${f.rule_code}`)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
