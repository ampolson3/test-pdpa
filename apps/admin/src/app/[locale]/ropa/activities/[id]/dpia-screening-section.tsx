"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import { type ApiClient, useDpiaAssessments, useScreenActivity } from "@pdpa/api-client";

const FACTOR_KEYS = ["sensitive_data", "large_scale", "monitoring", "automated_decision", "new_tech", "vulnerable_groups"] as const;

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

/** DPIA-01/02: screen this RoPA activity against the tenant's own thresholds, and show past rounds. */
export function DpiaScreeningSection({ client, activityId }: { client: ApiClient; activityId: string }) {
  const t = useTranslations("dpia");
  const locale = useLocale() as Locale;
  const canScreen = usePermission("assessment.dpia.create");
  const canRead = usePermission("assessment.dpia.read");
  const rounds = useDpiaAssessments(client, { activity_id: activityId });
  const screen = useScreenActivity(client);

  const [answers, setAnswers] = useState<Record<string, "yes" | "no">>(
    Object.fromEntries(FACTOR_KEYS.map((k) => [k, "no"])) as Record<string, "yes" | "no">,
  );

  if (!canRead) return null;

  return (
    <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="dpia-section">
      <h2 className="font-semibold">{t("sections.screening")}</h2>

      {rounds.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : rounds.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : (rounds.data?.pages?.flatMap((p) => p.data).length ?? 0) === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : (
        <ul className="divide-y divide-slate-100">
          {rounds.data!.pages.flatMap((p) => p.data).map((a) => (
            <li key={a.id} className="py-2">
              <span className="font-medium">{t("roundLabel", { round: a.round_no })}</span>
              {" — "}
              <span>{t(`results.${a.screening_result}`)}</span>
              <span className="text-slate-500"> ({formatDate(a.created_at, locale, { day: "numeric", month: "short", year: "numeric" })})</span>
              <p className="text-slate-500">{a.screening_reason}</p>
            </li>
          ))}
        </ul>
      )}

      {canScreen && (
        <div className="space-y-2 pt-2">
          <div className="grid gap-2 sm:grid-cols-2">
            {FACTOR_KEYS.map((key) => (
              <label key={key} className="flex items-start gap-2">
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={answers[key] === "yes"}
                  onChange={(e) => setAnswers({ ...answers, [key]: e.target.checked ? "yes" : "no" })}
                />
                <span>{t(`factors.${key}`)}</span>
              </label>
            ))}
          </div>
          {screen.isError && <p className="text-red-700">{t("submitError", { detail: detail(screen.error) })}</p>}
          <Button
            onClick={() => screen.mutate({ activityId, answers })}
            disabled={screen.isPending}
          >
            {t("submit")}
          </Button>
        </div>
      )}
    </section>
  );
}
