"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { formatDate, type Locale } from "@pdpa/i18n";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { FormRenderer, type FormSchema, type Language, type Scoring } from "@pdpa/form-renderer";
import { createApiClient, useForm, useFrameworks, useAssessFramework, useFrameworkAssessments } from "@pdpa/api-client";
import { useRendererMessages } from "@/components/form-messages";

function problemText(e: unknown): string {
  if (typeof e !== "object" || e === null) return "";
  const p = e as { title?: string; detail?: string; errors?: { field: string; code: string }[] };
  const fields = (p.errors ?? []).map((f) => `${f.field}: ${f.code}`).join(", ");
  return [p.title, p.detail, fields].filter(Boolean).join(" — ");
}

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

/** DPX-10: pick a related law/notice from the global catalog (migration 00096) and get its own
 *  ready-to-use checklist — answering it is scored by the same PLT-06 form engine every other
 *  checklist in this codebase already uses. */
export function FrameworksSection({ client }: { client: ReturnType<typeof createApiClient> }) {
  const t = useTranslations("governance");
  const locale = useLocale() as Language;
  const canRead = usePermission("dpx.framework.read");
  const canExecute = usePermission("dpx.framework.execute");
  const frameworks = useFrameworks(client);
  const [code, setCode] = useState("");
  const chosen = code || frameworks.data?.[0]?.code;
  const framework = (frameworks.data ?? []).find((f) => f.code === chosen);
  const form = useForm(client, framework?.checklist_form_id ?? undefined);
  const version = form.data?.versions?.find((v) => v.id === form.data?.current_version_id);
  const assess = useAssessFramework(client);
  const runs = useFrameworkAssessments(client, chosen ?? "");
  const messages = useRendererMessages(locale);

  if (!canRead) return null;

  return (
    <section className="space-y-3" data-testid="frameworks-section">
      <h2 className="text-lg font-semibold">{t("frameworks.title")}</h2>
      <p className="text-slate-600">{t("frameworks.intro")}</p>

      <select className={INPUT} value={chosen ?? ""} onChange={(e) => setCode(e.target.value)} data-testid="framework-picker">
        {(frameworks.data ?? []).map((f) => (
          <option key={f.code} value={f.code}>
            {locale === "th" ? f.name_th : f.name_en}
          </option>
        ))}
      </select>

      {framework && !framework.checklist_form_id && <p className="text-amber-800">{t("frameworks.noChecklist")}</p>}

      {canExecute && version && (
        <div className="space-y-3" data-testid="framework-checklist-form">
          <FormRenderer
            schema={version.schema as FormSchema}
            scoring={version.scoring as Scoring | null}
            language={locale}
            messages={messages}
            showScore
            idPrefix="framework-checklist"
            onSubmit={async (answers) => {
              await assess.mutateAsync({ code: chosen!, answers });
            }}
            actions={({ submit, busy }) => (
              <button
                type="button"
                onClick={submit}
                disabled={busy || assess.isPending}
                className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
                data-testid="framework-checklist-submit"
              >
                {t("frameworks.submit")}
              </button>
            )}
          />
          {assess.isError && (
            <p className="text-red-700" role="alert">
              {problemText(assess.error)}
            </p>
          )}
        </div>
      )}

      {runs.isPending ? (
        <p className="text-slate-500">{t("frameworks.loading")}</p>
      ) : (runs.data ?? []).length === 0 ? (
        <p className="rounded-md bg-slate-50 p-3 text-slate-600">{t("frameworks.empty")}</p>
      ) : (
        <ul className="space-y-1" data-testid="framework-assessment-runs">
          {(runs.data ?? []).map((r) => (
            <li key={r.id} className="flex items-center gap-2 text-sm">
              <span>{t("frameworks.score", { score: r.score })}</span>
              <span className="text-xs text-slate-500">
                {formatDate(r.assessed_at, locale as Locale, { month: "short", hour: "2-digit", minute: "2-digit" })}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
