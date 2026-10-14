"use client";

import { useActionState } from "react";
import { useTranslations } from "next-intl";
import type { BreachPublicReportForm as ReportForm, BreachType } from "@/lib/breach";
import { submitBreachReport, type SubmitState } from "./actions";

export function BreachReportForm({ form, publicKey, locale, idem }: { form: ReportForm; publicKey: string; locale: string; idem: string }) {
  const t = useTranslations("portal.breach");
  const [state, action, pending] = useActionState(submitBreachReport.bind(null, publicKey, locale), { idem } as SubmitState);

  if (state.incidentNo) {
    return (
      <section className="space-y-3 rounded-lg bg-white p-6 shadow-sm" role="status" data-testid="breach-submitted">
        <h1 className="text-lg font-semibold">{t("thanks")}</h1>
        <p>{t("incidentNo", { incidentNo: state.incidentNo })}</p>
        <p className="text-sm text-emerald-700">{t("notified")}</p>
      </section>
    );
  }

  return (
    <form action={action} className="space-y-5 rounded-lg bg-white p-6 shadow-sm">
      <header>
        <p className="text-sm text-slate-600">{t("intro")}</p>
      </header>

      <fieldset className="space-y-2">
        <legend className="font-medium">{t("breachType")}</legend>
        {form.breach_types.map((bt: BreachType) => (
          <label key={bt} className="flex items-start gap-2 text-sm">
            <input type="checkbox" name="breach_types" value={bt} className="mt-0.5" data-testid={`breach-type-${bt}`} />
            {t(`breachTypes.${bt}`)}
          </label>
        ))}
      </fieldset>

      <label className="block text-sm">
        {t("titleField")}
        <input name="title" required className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="title" />
      </label>

      <label className="block text-sm">
        {t("description")}
        <textarea name="description" required rows={4} className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="description" />
      </label>

      <label className="block text-sm">
        {t("occurredAt")}
        <input type="datetime-local" name="occurred_at" className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="occurred-at" />
      </label>

      <label className="block text-sm">
        {t("reporterContact")}
        <input name="reporter_contact" className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="reporter-contact" />
      </label>

      {state.error && (
        <p role="alert" className="text-sm text-rose-700" data-testid="error">
          {t.has(`errors.${state.error.code.replace(".", "_")}`) ? t(`errors.${state.error.code.replace(".", "_")}`) : t("errors.generic")}
        </p>
      )}

      <button type="submit" disabled={pending} className="rounded-md bg-blue-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-60" data-testid="submit">
        {pending ? t("submitting") : t("submit")}
      </button>
    </form>
  );
}
