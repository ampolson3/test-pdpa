"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useCloneDpiaTemplate,
  useCreateDpiaTemplate,
  useDpiaTemplates,
  usePublishDpiaTemplate,
  useRetireDpiaTemplate,
  type DpiaTemplate,
} from "@pdpa/api-client";
import { Link } from "@/i18n/routing";
import { problemCode } from "@/components/form-messages";

const ASSESSMENT_TYPES = [
  "dpia", "pia", "lia", "tia", "ai", "security", "maturity", "dpo_check", "sme_check", "vendor", "inbound_dpa", "independence",
];

const INPUT = "mt-1 w-full rounded border border-slate-300 px-2 py-1";

export function DpiaTemplatesContent() {
  const t = useTranslations("dpia");
  const canRead = usePermission("assessment.template.read");
  const canCreate = usePermission("assessment.template.create");
  const canPublish = usePermission("assessment.template.publish");
  const canRetire = usePermission("assessment.template.delete");
  const client = useMemo(() => createApiClient("/api/bff"), []);

  const [filterType, setFilterType] = useState("");
  const templates = useDpiaTemplates(client, filterType || undefined);
  const clone = useCloneDpiaTemplate(client);
  const publish = usePublishDpiaTemplate(client);
  const retire = useRetireDpiaTemplate(client);

  const [cloning, setCloning] = useState<DpiaTemplate | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("templates.forbidden")}</main>;

  return (
    <main className="mx-auto max-w-5xl space-y-6 p-8 text-sm">
      <h1 className="text-xl font-semibold">{t("templates.title")}</h1>
      <p className="text-slate-500">{t("templates.intro")}</p>

      <div className="flex items-center gap-3">
        <label className="block">
          <span className="block text-slate-600">{t("templates.assessmentType")}</span>
          <select className={INPUT} value={filterType} onChange={(e) => setFilterType(e.target.value)}>
            <option value="">{t("templates.filterAll")}</option>
            {ASSESSMENT_TYPES.map((x) => (
              <option key={x} value={x}>{t(`templates.types.${x}`)}</option>
            ))}
          </select>
        </label>
      </div>

      {actionError && <p role="alert" className="text-red-700">{actionError}</p>}

      {templates.isPending ? (
        <p className="text-slate-500">{t("templates.loading")}</p>
      ) : templates.isError ? (
        <p className="text-red-700">{t("templates.loadError")}</p>
      ) : (templates.data?.data.length ?? 0) === 0 ? (
        <p className="text-slate-500">{t("templates.empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr>
              <th className="px-3 py-2">{t("templates.name")}</th>
              <th className="px-3 py-2">{t("templates.assessmentType")}</th>
              <th className="px-3 py-2">{t("templates.status")}</th>
              <th className="px-3 py-2">{t("templates.version")}</th>
              <th className="px-3 py-2" />
            </tr>
          </thead>
          <tbody>
            {templates.data!.data.map((tpl) => (
              <tr key={tpl.id} className="border-t border-slate-100">
                <td className="px-3 py-2">
                  <Link href={`/forms/${tpl.form_id}`} className="font-medium underline">{tpl.name}</Link>
                  <span className="ml-2 text-xs text-slate-500">{tpl.code}</span>
                </td>
                <td className="px-3 py-2">{t(`templates.types.${tpl.assessment_type}`)}</td>
                <td className="px-3 py-2">{t(`templates.statusValues.${tpl.status}`)}</td>
                <td className="px-3 py-2">{tpl.version_no}</td>
                <td className="space-x-2 px-3 py-2 text-right">
                  {canCreate && (
                    <Button variant="secondary" onClick={() => setCloning(tpl)}>{t("templates.clone")}</Button>
                  )}
                  {canPublish && tpl.status === "draft" && (
                    <Button
                      variant="secondary"
                      onClick={() =>
                        publish.mutate(
                          { id: tpl.id, ifMatch: String(tpl.row_version) },
                          { onError: () => setActionError(t("templates.actionError")) },
                        )
                      }
                      disabled={publish.isPending}
                    >
                      {t("templates.publish")}
                    </Button>
                  )}
                  {canRetire && tpl.status !== "retired" && (
                    <Button
                      variant="secondary"
                      onClick={() =>
                        retire.mutate(
                          { id: tpl.id, ifMatch: String(tpl.row_version) },
                          { onError: () => setActionError(t("templates.actionError")) },
                        )
                      }
                      disabled={retire.isPending}
                    >
                      {t("templates.retire")}
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {cloning && (
        <CloneDialog
          source={cloning}
          onClose={() => setCloning(null)}
          onClone={(code, name) =>
            clone.mutate(
              { id: cloning.id, code, name },
              {
                onSuccess: () => setCloning(null),
                onError: () => setActionError(t("templates.actionError")),
              },
            )
          }
          pending={clone.isPending}
        />
      )}

      {canCreate && <CreateTemplate client={client} />}
    </main>
  );
}

function CloneDialog({
  source,
  onClose,
  onClone,
  pending,
}: {
  source: DpiaTemplate;
  onClose: () => void;
  onClone: (code: string, name: string) => void;
  pending: boolean;
}) {
  const t = useTranslations("dpia");
  const [code, setCode] = useState(`${source.code}_copy`);
  const [name, setName] = useState(`${source.name} (copy)`);
  return (
    <div className="fixed inset-0 flex items-center justify-center bg-black/30" role="dialog" aria-modal="true">
      <div className="w-full max-w-sm space-y-3 rounded-md bg-white p-4 shadow-lg">
        <h2 className="text-base font-semibold">{t("templates.cloneDialog.title")}</h2>
        <label className="block">
          <span className="block text-slate-600">{t("templates.cloneDialog.code")}</span>
          <input className={INPUT} value={code} onChange={(e) => setCode(e.target.value)} pattern="[a-z][a-z0-9_]*" />
        </label>
        <label className="block">
          <span className="block text-slate-600">{t("templates.cloneDialog.name")}</span>
          <input className={INPUT} value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>{t("templates.cloneDialog.cancel")}</Button>
          <Button onClick={() => onClone(code, name)} disabled={pending || !code || !name.trim()}>
            {t("templates.cloneDialog.submit")}
          </Button>
        </div>
      </div>
    </div>
  );
}

function CreateTemplate({ client }: { client: ReturnType<typeof createApiClient> }) {
  const t = useTranslations("dpia");
  const create = useCreateDpiaTemplate(client);
  const [assessmentType, setAssessmentType] = useState(ASSESSMENT_TYPES[0]!);
  const [code, setCode] = useState("");
  const [name, setName] = useState("");

  const submit = () =>
    create.mutate({
      assessment_type: assessmentType,
      code,
      name,
      draft: {
        schema: {
          sections: [
            { key: "section_1", title: { th: name }, questions: [{ key: "question_1", type: "yes_no", label: { th: name } }] },
          ],
        },
      },
    });

  const err = create.error ? (problemCode(create.error) === "dpia.invalid_input" ? t("templates.create.invalid") : t("templates.actionError")) : null;

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4" aria-labelledby="create-template">
      <h2 id="create-template" className="text-base font-semibold">{t("templates.create.title")}</h2>
      <div className="grid gap-3 sm:grid-cols-3">
        <label className="block">
          <span className="block text-slate-600">{t("templates.assessmentType")}</span>
          <select className={INPUT} value={assessmentType} onChange={(e) => setAssessmentType(e.target.value)}>
            {ASSESSMENT_TYPES.map((x) => (
              <option key={x} value={x}>{t(`templates.types.${x}`)}</option>
            ))}
          </select>
        </label>
        <div>
          <label className="block">
            <span className="block text-slate-600">{t("templates.code")}</span>
            <input className={`${INPUT} font-mono`} value={code} onChange={(e) => setCode(e.target.value)} pattern="[a-z][a-z0-9_]*" aria-describedby="template-code-help" />
          </label>
          <p id="template-code-help" className="text-xs text-slate-500">{t("templates.create.codeHelp")}</p>
        </div>
        <label className="block">
          <span className="block text-slate-600">{t("templates.name")}</span>
          <input className={INPUT} value={name} onChange={(e) => setName(e.target.value)} />
        </label>
      </div>
      <Button onClick={submit} disabled={create.isPending || !code || !name.trim()}>{t("templates.create.submit")}</Button>
      {err && <p role="alert" className="text-red-700">{err}</p>}
    </section>
  );
}
