"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { formatDate, type Locale } from "@pdpa/i18n";
import { usePermission } from "@pdpa/authz";
import { FormRenderer, type FormSchema, type Language, type Scoring } from "@pdpa/form-renderer";
import {
  createApiClient,
  useForm,
  useForms,
  useActivityChecklists,
  useActivityMutations,
} from "@pdpa/api-client";
import { useRendererMessages } from "@/components/form-messages";

function problemText(e: unknown): string {
  if (typeof e !== "object" || e === null) return "";
  const p = e as { title?: string; detail?: string; errors?: { field: string; code: string }[] };
  const fields = (p.errors ?? []).map((f) => `${f.field}: ${f.code}`).join(", ");
  return [p.title, p.detail, fields].filter(Boolean).join(" — ");
}

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

/** RRA-05: answer a published per-activity checklist and see past runs. Any item worth asking the
 *  DPO or a consultant about is a comment on this same activity (ROPA-12's own PLT-07 Q&A), not a
 *  separate channel — the checklist's own answers and any such question both end up stored against
 *  the activity. */
export function ActivityChecklistSection({ client, activityId }: { client: ReturnType<typeof createApiClient>; activityId: string }) {
  const t = useTranslations("activities");
  const locale = useLocale() as Language;
  const canRead = usePermission("ropa.risk.read");
  const canAnswer = usePermission("ropa.risk.update");
  const runs = useActivityChecklists(client, activityId);
  const forms = useForms(client, "checklist");
  const published = (forms.data ?? []).filter((f) => f.current_version_id);
  const [formId, setFormId] = useState("");
  const chosen = formId || published[0]?.id;
  const form = useForm(client, chosen);
  const m = useActivityMutations(client, activityId);
  const messages = useRendererMessages(locale);
  const version = form.data?.versions?.find((v) => v.id === form.data?.current_version_id);

  if (!canRead) return null;
  const rows = runs.data?.data ?? [];

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="activity-checklist-section">
      <h2 className="font-semibold">{t("sections.checklist")}</h2>

      {canAnswer && (
        <div className="space-y-3" data-testid="checklist-form">
          {published.length === 0 ? (
            <p className="text-amber-800">{t("checklist.noForm")}</p>
          ) : (
            <select className={INPUT} value={chosen ?? ""} onChange={(e) => setFormId(e.target.value)}>
              {published.map((f) => (
                <option key={f.id} value={f.id}>
                  {f.name}
                </option>
              ))}
            </select>
          )}
          {version && (
            <FormRenderer
              schema={version.schema as FormSchema}
              scoring={version.scoring as Scoring | null}
              language={locale}
              messages={messages}
              showScore
              idPrefix="activity-checklist"
              onSubmit={async (answers) => {
                await m.answerChecklist.mutateAsync({ form_id: chosen!, answers });
              }}
              actions={({ submit, busy }) => (
                <button
                  type="button"
                  onClick={submit}
                  disabled={busy || m.answerChecklist.isPending}
                  className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
                  data-testid="checklist-submit"
                >
                  {t("checklist.submit")}
                </button>
              )}
            />
          )}
          {m.answerChecklist.isError && (
            <p className="text-red-700" role="alert">
              {problemText(m.answerChecklist.error)}
            </p>
          )}
        </div>
      )}

      {runs.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : rows.length === 0 ? (
        <p className="rounded-md bg-slate-50 p-3 text-slate-600">{t("checklist.empty")}</p>
      ) : (
        <ul className="space-y-1" data-testid="checklist-runs">
          {rows.map((r) => (
            <li key={r.id} className="flex items-center gap-2 text-sm">
              <span className="rounded bg-slate-100 px-2 py-0.5 text-xs">{r.result}</span>
              <span>{t("checklist.score", { score: r.score })}</span>
              <span className="text-xs text-slate-500">{formatDate(r.completed_at, locale as Locale, { month: "short", hour: "2-digit", minute: "2-digit" })}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
