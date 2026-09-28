"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import { type ApiClient, useDpiaAssessmentDescription, useDpiaAssessments, useScreenActivity } from "@pdpa/api-client";

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

  const latest = rounds.data?.pages?.flatMap((p) => p.data)[0];

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

      {latest && latest.status === "in_progress" && <DpiaDescriptionPanel client={client} assessmentId={latest.id} />}

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

/** DPIA-04: the in-progress assessment's processing description, composed live from this RoPA activity. */
function DpiaDescriptionPanel({ client, assessmentId }: { client: ApiClient; assessmentId: string }) {
  const t = useTranslations("dpia");
  const locale = useLocale();
  const nameTh = (o: unknown, key: string): string => {
    const r = o as Record<string, string | undefined>;
    return (locale === "en" ? r[`${key}_en`] : undefined) ?? r[`${key}_th`] ?? "";
  };
  const desc = useDpiaAssessmentDescription(client, assessmentId);

  if (desc.isPending) return <p className="text-slate-500">{t("loading")}</p>;
  if (desc.isError) return <p className="text-red-700">{t("loadError")}</p>;
  const d = desc.data!;

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-slate-50 p-3 text-sm" data-testid="dpia-description">
      <h3 className="font-semibold">{t("description.title")}</h3>
      {d.activity_description && <p className="text-slate-600">{d.activity_description}</p>}

      <div>
        <h4 className="font-medium">{t("description.purposes")}</h4>
        <ul className="list-inside list-disc">
          {d.purposes.map((p, i) => (
            <li key={i}>
              {p.text} ({nameTh(p, "lawful_basis_name")})
            </li>
          ))}
        </ul>
      </div>

      <div>
        <h4 className="font-medium">{t("description.data")}</h4>
        <ul className="list-inside list-disc">
          {d.data.map((x, i) => (
            <li key={i}>
              {nameTh(x, "category_name")} — {nameTh(x, "subject_type_name")}
              {x.is_sensitive && <span className="ml-1 rounded bg-amber-100 px-1 text-xs text-amber-800">{t("description.sensitive")}</span>}
            </li>
          ))}
        </ul>
      </div>

      <div>
        <h4 className="font-medium">{t("description.recipients")}</h4>
        <ul className="list-inside list-disc">
          {d.recipients.map((r, i) => (
            <li key={i}>
              {nameTh(r, "party_name")} ({r.recipient_role})
            </li>
          ))}
        </ul>
      </div>

      <div>
        <h4 className="font-medium">{t("description.transfers")}</h4>
        <ul className="list-inside list-disc">
          {d.transfers.map((tr, i) => (
            <li key={i}>
              {nameTh(tr, "country_name")} — {tr.transfer_basis}
            </li>
          ))}
        </ul>
      </div>

      <div>
        <h4 className="font-medium">{t("description.retention")}</h4>
        <ul className="list-inside list-disc">
          {d.retention.map((r, i) => (
            <li key={i}>
              {nameTh(r, "category_name")}: {r.retention_months ? t("description.months", { count: r.retention_months }) : r.retention_basis} —{" "}
              {r.disposal_method}
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}
