"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@pdpa/ui";
import { FormRenderer, type FormSchema, type Language, type Scoring } from "@pdpa/form-renderer";
import {
  type ApiClient,
  type ActivityTransfer,
  useDpiaTemplates,
  useForm,
  useActivityMutations,
} from "@pdpa/api-client";
import { useRendererMessages } from "@/components/form-messages";

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

const SELECT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

/** DPX-06: run a transfer impact assessment (TIA) for one logged cross-border transfer (ม.28-29) — shown
 *  inline under a transfer row that has no tia_assessment_id yet. Reuses VEN-04/DPIA-03's own generic
 *  template library (assessment_type='tia') and FormRenderer, the exact pattern VEN-07/VEN-08 already set. */
export function TiaPanel({ client, activityId, transfer, editable }: { client: ApiClient; activityId: string; transfer: ActivityTransfer; editable: boolean }) {
  const t = useTranslations("activities");
  const locale = useLocale() as Language;
  const templates = useDpiaTemplates(client, "tia");
  const published = (templates.data?.data ?? []).filter((tpl) => tpl.status === "published");
  const [templateId, setTemplateId] = useState("");
  const chosen = templateId || published[0]?.id;
  const chosenTemplate = published.find((tpl) => tpl.id === chosen);
  const form = useForm(client, chosenTemplate?.form_id);
  const m = useActivityMutations(client, activityId);
  const messages = useRendererMessages(locale);
  const version = form.data?.versions?.find((ver) => ver.id === form.data?.current_version_id);

  if (transfer.tia_assessment_id) {
    return <span className="text-xs text-emerald-700">{t("tia.recorded")}</span>;
  }
  if (!editable) {
    return <span className="text-xs text-amber-700">{t("tia.missing")}</span>;
  }
  if (published.length === 0) {
    return <span className="text-xs text-amber-700">{t("tia.noTemplate")}</span>;
  }

  return (
    <div className="mt-2 space-y-2 rounded-md bg-slate-50 p-2" data-testid={`tia-form-${transfer.id}`}>
      <select className={SELECT} value={chosen ?? ""} onChange={(e) => setTemplateId(e.target.value)}>
        {published.map((tpl) => <option key={tpl.id} value={tpl.id}>{tpl.name}</option>)}
      </select>
      {version && chosenTemplate && (
        <FormRenderer schema={version.schema as FormSchema} scoring={version.scoring as Scoring | null} language={locale} messages={messages} showScore
          idPrefix={`tia-${transfer.id}`}
          onSubmit={async (answers) => {
            await m.recordTransferTia.mutateAsync({ transferId: transfer.id, input: { template_code: chosenTemplate.code, answers } });
          }}
          actions={({ submit, busy }) => <Button type="button" onClick={submit} disabled={busy || m.recordTransferTia.isPending} data-testid={`tia-submit-${transfer.id}`}>{t("tia.submit")}</Button>} />
      )}
      {m.recordTransferTia.isError && <p className="text-red-700" role="alert">{t("tia.submitError", { detail: detail(m.recordTransferTia.error) })}</p>}
    </div>
  );
}
