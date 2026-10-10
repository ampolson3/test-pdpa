"use client";

import { useMemo } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { createApiClient, useGapRules, useOpenGapFindings } from "@pdpa/api-client";

const SEVERITY_STYLE: Record<string, string> = {
  low: "bg-slate-100 text-slate-700",
  medium: "bg-amber-100 text-amber-800",
  high: "bg-red-100 text-red-800",
};

/** RRA-04's own register screen: every open legal-gap finding across every processing activity,
 *  from the last analysis run on each one, each linking straight to the activity that has it —
 *  the backlog's own "รายการช่องว่าง + ลิงก์ไปแก้". Re-running the analysis itself happens on the
 *  activity page (GapAnalysisSection); this page is read-only. */
export function GapRegisterContent() {
  const t = useTranslations("gapRegister");
  const canRead = usePermission("ropa.risk.read");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const findings = useOpenGapFindings(client);
  const rules = useGapRules(client);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const ruleByCode = new Map((rules.data ?? []).map((r) => [r.code, r]));
  const rows = (findings.data ?? []).filter((f) => f.status === "open");

  return (
    <main className="mx-auto max-w-5xl space-y-6 p-8 text-sm">
      <header>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-slate-600">{t("intro")}</p>
      </header>

      {findings.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : findings.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : rows.length === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : (
        <table className="w-full border-collapse text-left" data-testid="gap-register-table">
          <thead>
            <tr className="border-b border-slate-200 text-slate-500">
              <th className="py-2">{t("table.rule")}</th>
              <th className="py-2">{t("table.severity")}</th>
              <th className="py-2">{t("table.detectedAt")}</th>
              <th className="py-2" />
            </tr>
          </thead>
          <tbody>
            {rows.map((f) => {
              const rule = ruleByCode.get(f.rule_code);
              return (
                <tr key={f.id} className="border-b border-slate-100">
                  <td className="py-2">{rule?.name ?? f.rule_code}</td>
                  <td className="py-2">
                    <span className={`rounded px-2 py-0.5 text-xs font-semibold ${SEVERITY_STYLE[rule?.severity ?? ""] ?? "bg-slate-100 text-slate-700"}`}>
                      {rule ? t(`severity.${rule.severity}`) : f.status}
                    </span>
                  </td>
                  <td className="py-2">{new Date(f.detected_at).toLocaleString()}</td>
                  <td className="py-2">
                    <Link className="text-sky-700 underline" href={`/ropa/activities/${f.activity_id}`}>
                      {t("viewActivity")}
                    </Link>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </main>
  );
}
