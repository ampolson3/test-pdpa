"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { Link } from "@/i18n/routing";
import { formatDate, type Locale } from "@pdpa/i18n";
import {
  type ApiClient,
  type DpiaOpinionRecommendation,
  useAssessNecessity,
  useDpiaAssessmentDescription,
  useDpiaAssessmentDiff,
  useDpiaAssessments,
  useDpiaNecessity,
  useDpiaOpinions,
  useRecordDpiaOpinion,
  useScreenActivity,
  useTransitionDpiaAssessment,
  dpiaReportHref,
} from "@pdpa/api-client";
import { RecordCollaboration } from "@/components/record-collaboration";

const NECESSITY_KEYS = ["minimal_data", "purpose_specific", "lawful_basis_appropriate", "less_invasive_considered"] as const;

const FACTOR_KEYS = ["sensitive_data", "large_scale", "monitoring", "automated_decision", "new_tech", "vulnerable_groups"] as const;

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

/** DPIA-01/02: screen this RoPA activity against the tenant's own thresholds, and show past rounds. */
export function DpiaScreeningSection({ client, activityId, currentUserId }: { client: ApiClient; activityId: string; currentUserId: string }) {
  const t = useTranslations("dpia");
  const locale = useLocale() as Locale;
  const canScreen = usePermission("assessment.dpia.create");
  const canUpdate = usePermission("assessment.dpia.update");
  const canRead = usePermission("assessment.dpia.read");
  const rounds = useDpiaAssessments(client, { activity_id: activityId });
  const screen = useScreenActivity(client);
  const [historyId, setHistoryId] = useState<string | null>(null);

  const [answers, setAnswers] = useState<Record<string, "yes" | "no">>(
    Object.fromEntries(FACTOR_KEYS.map((k) => [k, "no"])) as Record<string, "yes" | "no">,
  );

  const latest = rounds.data?.pages?.flatMap((p) => p.data)[0];

  if (!canRead) return null;

  return (
    <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="dpia-section">
      <div className="flex items-center justify-between">
        <h2 className="font-semibold">{t("sections.screening")}</h2>
        <Link className="text-sm text-sky-700 underline" href="/dpia-register">{t("registry.link")}</Link>
      </div>

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
              {a.round_no > 1 && <DpiaDiffPanel client={client} assessmentId={a.id} />}
              <div className="flex flex-wrap gap-2 py-1 text-xs">
                {(["th", "en"] as const).flatMap((l) =>
                  (["pdf", "docx"] as const).map((f) => (
                    <a key={`${l}${f}`} className="text-sky-700 underline" data-testid={`dpia-report-${a.id}-${l}-${f}`}
                      href={dpiaReportHref("/api/bff", a.id, l, f)} target={f === "pdf" ? "_blank" : undefined} rel="noreferrer">
                      {t("report.download", { format: f === "pdf" ? "PDF" : "Word", lang: t(`report.lang.${l}`) })}
                    </a>
                  )),
                )}
              </div>
              <button
                type="button"
                className="text-sky-700 underline"
                data-testid={`dpia-history-toggle-${a.id}`}
                onClick={() => setHistoryId(historyId === a.id ? null : a.id)}
              >
                {historyId === a.id ? t("form.hide") : t("history")}
              </button>
              {historyId === a.id && (
                <RecordCollaboration entityType="dpia_assessment" entityId={a.id} canWrite={canUpdate} currentUserId={currentUserId} />
              )}
            </li>
          ))}
        </ul>
      )}

      {latest && latest.status === "in_progress" && (
        <>
          <DpiaDescriptionPanel client={client} assessmentId={latest.id} />
          <DpiaNecessityPanel client={client} assessmentId={latest.id} />
        </>
      )}

      {latest && latest.status !== "screening" && latest.status !== "closed" && (
        <DpiaDecisionPanel client={client} assessment={latest} />
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

/** DPIA-14: what changed in this round's screening answers versus the round it supersedes. */
function DpiaDiffPanel({ client, assessmentId }: { client: ApiClient; assessmentId: string }) {
  const t = useTranslations("dpia");
  const diff = useDpiaAssessmentDiff(client, assessmentId);

  if (diff.isPending) return null;
  if (diff.isError) return <p className="text-red-700">{t("loadError")}</p>;
  const changes = diff.data?.changes ?? [];
  if (changes.length === 0) return null;

  return (
    <div className="mt-1 rounded bg-sky-50 p-2 text-xs" data-testid={`dpia-diff-${assessmentId}`}>
      <p className="font-medium text-sky-800">{t("diff.title")}</p>
      <ul className="list-inside list-disc">
        {changes.map((c) => (
          <li key={c.question}>
            {t(`factors.${c.question}`)}: {String(c.before ?? t("diff.blank"))} → {String(c.after ?? t("diff.blank"))}
          </li>
        ))}
      </ul>
    </div>
  );
}

/** DPIA-05: answer (or re-answer) the necessity/proportionality checklist for the in-progress assessment. */
function DpiaNecessityPanel({ client, assessmentId }: { client: ApiClient; assessmentId: string }) {
  const t = useTranslations("dpia");
  const canAssess = usePermission("assessment.dpia.update");
  const canRead = usePermission("assessment.dpia.read");
  const necessity = useDpiaNecessity(client, assessmentId);
  const assess = useAssessNecessity(client);

  const [answers, setAnswers] = useState<Record<string, "yes" | "no">>(
    Object.fromEntries(NECESSITY_KEYS.map((k) => [k, "no"])) as Record<string, "yes" | "no">,
  );

  if (!canRead) return null;

  const notFound = necessity.isError && (necessity.error as { code?: string } | undefined)?.code === "not_found";
  const otherError = necessity.isError && !notFound;

  return (
    <section className="space-y-2 rounded-md border border-slate-200 bg-white p-3 text-sm" data-testid="dpia-necessity">
      <h3 className="font-semibold">{t("necessity.title")}</h3>

      {necessity.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : necessity.data ? (
        <div className="space-y-1">
          <p>
            <span className={necessity.data.result === "necessary" ? "text-green-700" : "text-amber-700"}>
              {t(`necessity.results.${necessity.data.result}`)}
            </span>
          </p>
          {necessity.data.missing.length > 0 && (
            <ul className="list-inside list-disc text-slate-600">
              {necessity.data.missing.map((k) => (
                <li key={k}>{t(`necessity.questions.${k}`)}</li>
              ))}
            </ul>
          )}
        </div>
      ) : otherError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : (
        <p className="text-slate-500">{t("necessity.notAnswered")}</p>
      )}

      {canAssess && (
        <div className="space-y-2 pt-2">
          <div className="grid gap-2 sm:grid-cols-2">
            {NECESSITY_KEYS.map((key) => (
              <label key={key} className="flex items-start gap-2">
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={answers[key] === "yes"}
                  onChange={(e) => setAnswers({ ...answers, [key]: e.target.checked ? "yes" : "no" })}
                />
                <span>{t(`necessity.questions.${key}`)}</span>
              </label>
            ))}
          </div>
          {assess.isError && <p className="text-red-700">{t("submitError", { detail: detail(assess.error) })}</p>}
          <Button onClick={() => assess.mutate({ assessmentId, answers })} disabled={assess.isPending}>
            {t("necessity.submit")}
          </Button>
        </div>
      )}
    </section>
  );
}

const RECOMMENDATIONS: DpiaOpinionRecommendation[] = ["proceed", "proceed_with_conditions", "do_not_proceed", "consult_pdpc"];

/** DPIA-10: submit for review, the DPO's opinion, the decision (approved/rejected/needs_review), resume
 *  and close (ST-05#2) — shown for any round past screening that hasn't closed yet. */
function DpiaDecisionPanel({ client, assessment }: { client: ApiClient; assessment: { id: string; status: string; row_version: number } }) {
  const t = useTranslations("dpia");
  const canUpdate = usePermission("assessment.dpia.update");
  const canApprove = usePermission("assessment.dpia.approve");
  const opinions = useDpiaOpinions(client, assessment.id);
  const recordOpinion = useRecordDpiaOpinion(client);
  const transition = useTransitionDpiaAssessment(client);
  const [opinionText, setOpinionText] = useState("");
  const [recommendation, setRecommendation] = useState<DpiaOpinionRecommendation>("proceed");
  const [reason, setReason] = useState("");

  const go = (to: "in_review" | "in_progress" | "approved" | "rejected" | "needs_review" | "closed") =>
    transition.mutate({ assessmentId: assessment.id, ifMatch: `"${assessment.row_version}"`, to, reason });

  return (
    <section className="space-y-2 rounded-md border border-slate-200 bg-white p-3 text-sm" data-testid="dpia-decision">
      <h3 className="font-semibold">{t("decision.title")}</h3>
      <p className="text-slate-500">{t(`decision.status.${assessment.status}`)}</p>
      {transition.isError && <p className="text-red-700">{t("submitError", { detail: detail(transition.error) })}</p>}

      {canUpdate && assessment.status === "in_progress" && <Button onClick={() => go("in_review")}>{t("decision.submitForReview")}</Button>}

      {assessment.status === "in_review" && (
        <>
          <div className="space-y-1">
            <h4 className="font-medium">{t("decision.opinions")}</h4>
            {opinions.data?.data.length ? (
              <ul className="list-inside list-disc text-slate-600">
                {opinions.data.data.map((o) => (
                  <li key={o.id}>
                    {o.opinion} — <span className="italic">{t(`decision.recommendation.${o.recommendation}`)}</span>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-slate-500">{t("decision.noOpinions")}</p>
            )}
            {canUpdate && (
              <div className="space-y-2 pt-1">
                <textarea
                  className="w-full rounded border border-slate-300 p-2"
                  value={opinionText}
                  onChange={(e) => setOpinionText(e.target.value)}
                  placeholder={t("decision.opinionPlaceholder")}
                />
                <select
                  className="rounded border border-slate-300 p-1"
                  value={recommendation}
                  onChange={(e) => setRecommendation(e.target.value as DpiaOpinionRecommendation)}
                >
                  {RECOMMENDATIONS.map((r) => (
                    <option key={r} value={r}>
                      {t(`decision.recommendation.${r}`)}
                    </option>
                  ))}
                </select>
                {recordOpinion.isError && <p className="text-red-700">{t("submitError", { detail: detail(recordOpinion.error) })}</p>}
                <Button
                  onClick={() => recordOpinion.mutate({ assessmentId: assessment.id, opinion: opinionText, recommendation })}
                  disabled={recordOpinion.isPending || !opinionText.trim()}
                >
                  {t("decision.recordOpinion")}
                </Button>
              </div>
            )}
          </div>

          {canApprove && (
            <div className="space-y-2 border-t border-slate-100 pt-2">
              <input
                className="w-full rounded border border-slate-300 p-2"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                placeholder={t("decision.reasonPlaceholder")}
              />
              <div className="flex gap-2">
                <Button onClick={() => go("approved")}>{t("decision.approve")}</Button>
                <Button onClick={() => go("needs_review")}>{t("decision.needsReview")}</Button>
                <Button onClick={() => go("rejected")}>{t("decision.reject")}</Button>
              </div>
            </div>
          )}
        </>
      )}

      {canUpdate && assessment.status === "needs_review" && <Button onClick={() => go("in_progress")}>{t("decision.resume")}</Button>}

      {canApprove && ["approved", "rejected", "not_required"].includes(assessment.status) && (
        <Button onClick={() => go("closed")}>{t("decision.close")}</Button>
      )}
    </section>
  );
}
