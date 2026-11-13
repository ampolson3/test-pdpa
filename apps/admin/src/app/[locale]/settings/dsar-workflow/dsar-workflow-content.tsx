"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import {
  createApiClient,
  useDsarRequestTypes,
  useDsarSaveRequestTypeConfig,
  useDsarResponseTemplates,
  useDsarSaveResponseTemplate,
  useMentionSearch,
  type DsarRequestType,
  type DsarSubtaskAction,
} from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const STEPS: DsarSubtaskAction[] = ["search", "export", "stop_marketing", "delete", "restrict", "rectify", "review"];

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

function ResponsiblePicker({ client, onPick }: { client: ReturnType<typeof createApiClient>; onPick: (u: { id: string; display_name: string }) => void }) {
  const t = useTranslations("dsarWorkflow");
  const [q, setQ] = useState("");
  const matches = useMentionSearch(client, q.length >= 2 ? q : null);
  return (
    <div className="relative inline-block w-full max-w-xs">
      <input className={INPUT} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("responsibleSearch")} />
      {q.length >= 2 && !!matches.data?.length && (
        <ul className="absolute z-10 w-full rounded-md border border-slate-200 bg-white text-sm shadow">
          {matches.data.map((u) => (
            <li key={u.id}>
              <button type="button" className="block w-full px-3 py-1.5 text-left hover:bg-slate-50" onClick={() => { onPick(u); setQ(""); }}>{u.display_name}</button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function TypeRow({ client, rt, canUpdate }: { client: ReturnType<typeof createApiClient>; rt: DsarRequestType; canUpdate: boolean }) {
  const t = useTranslations("dsarWorkflow");
  const save = useDsarSaveRequestTypeConfig(client);
  const [action, setAction] = useState(rt.default_subtask_action ?? "");
  const [assigneeId, setAssigneeId] = useState(rt.default_assignee_user_id ?? "");
  const [assigneeName, setAssigneeName] = useState("");
  const [result, setResult] = useState(rt.response_code_result ?? "");
  const [rejection, setRejection] = useState(rt.response_code_rejection ?? "");
  const [requestInfo, setRequestInfo] = useState(rt.response_code_request_info ?? "");
  const [savedAt, setSavedAt] = useState(0);
  const [error, setError] = useState("");

  const onSave = () => {
    setError("");
    save.mutate(
      {
        code: rt.code,
        input: {
          default_subtask_action: action ? (action as DsarSubtaskAction) : null,
          default_assignee_user_id: assigneeId || null,
          response_code_result: result.trim() || null,
          response_code_rejection: rejection.trim() || null,
          response_code_request_info: requestInfo.trim() || null,
        },
      },
      { onSuccess: () => setSavedAt(Date.now()), onError: (e) => setError(detail(e)) },
    );
  };

  return (
    <tr className="border-t border-slate-100 align-top" data-testid={`type-row-${rt.code}`}>
      <td className="py-2 pr-4 font-medium">{rt.name_th}<br /><span className="text-xs text-slate-500">{rt.code}</span></td>
      <td className="py-2 pr-4">
        <select value={action} onChange={(e) => setAction(e.target.value)} disabled={!canUpdate} className={INPUT} data-testid={`type-step-${rt.code}`}>
          <option value="">{t("stepPlaceholder")}</option>
          {STEPS.map((s) => (
            <option key={s} value={s}>{t(`steps.${s}`)}</option>
          ))}
        </select>
      </td>
      <td className="py-2 pr-4">
        {canUpdate ? (
          <div className="space-y-1">
            <ResponsiblePicker client={client} onPick={(u) => { setAssigneeId(u.id); setAssigneeName(u.display_name); }} />
            {assigneeId && (
              <p className="text-xs text-slate-600">
                {assigneeName || assigneeId}{" "}
                <button type="button" className="text-rose-700 underline" onClick={() => { setAssigneeId(""); setAssigneeName(""); }}>{t("clearResponsible")}</button>
              </p>
            )}
          </div>
        ) : (
          <span className="text-xs text-slate-500">{assigneeId || "—"}</span>
        )}
      </td>
      <td className="py-2 pr-4 space-y-1">
        <input value={result} onChange={(e) => setResult(e.target.value)} disabled={!canUpdate} placeholder={t("templatePlaceholder")} className={INPUT} data-testid={`type-result-${rt.code}`} />
        <input value={rejection} onChange={(e) => setRejection(e.target.value)} disabled={!canUpdate} placeholder={t("templatePlaceholder")} className={INPUT} />
        <input value={requestInfo} onChange={(e) => setRequestInfo(e.target.value)} disabled={!canUpdate} placeholder={t("templatePlaceholder")} className={INPUT} />
      </td>
      <td className="py-2 text-right">
        {canUpdate && (
          <button type="button" onClick={onSave} disabled={save.isPending} className="rounded-md bg-blue-700 px-3 py-1.5 text-sm text-white disabled:opacity-60" data-testid={`type-save-${rt.code}`}>
            {t("save")}
          </button>
        )}
        {error && <p className="mt-1 text-xs text-rose-700" role="alert">{error}</p>}
        {!error && savedAt > 0 && <p className="mt-1 text-xs text-emerald-700">{t("saved")}</p>}
      </td>
    </tr>
  );
}

export function DsarWorkflowContent() {
  const t = useTranslations("dsarWorkflow");
  const canRead = usePermission("dsar.form.read");
  const canUpdate = usePermission("dsar.form.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const types = useDsarRequestTypes(client);
  const templates = useDsarResponseTemplates(client);
  const saveTemplate = useDsarSaveResponseTemplate(client);
  const [code, setCode] = useState("");
  const [language, setLanguage] = useState<"th" | "en">("th");
  const [text, setText] = useState("");
  const [tplError, setTplError] = useState("");

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = types.data ?? [];
  const overrides = templates.data ?? [];

  const onSaveTemplate = () => {
    if (!code.trim() || !text.trim()) return;
    setTplError("");
    saveTemplate.mutate(
      { code: code.trim(), input: { language, content: { type: "doc", content: [{ type: "paragraph", content: [{ type: "text", text: text.trim() }] }] } } },
      { onSuccess: () => setText(""), onError: (e) => setTplError(detail(e)) },
    );
  };

  return (
    <main className="mx-auto max-w-6xl space-y-8 p-4 sm:p-8">
      <h1 className="text-xl font-semibold">{t("title")}</h1>
      <p className="text-sm text-slate-600">{t("intro")}</p>

      <section className="space-y-2 rounded-lg border border-slate-200 bg-white p-4">
        <h2 className="font-medium">{t("typesTitle")}</h2>
        <table className="w-full text-sm">
          <thead className="text-left text-slate-500">
            <tr>
              <th className="py-1 pr-4">{t("code")}</th>
              <th className="pr-4">{t("step")}</th>
              <th className="pr-4">{t("responsible")}</th>
              <th className="pr-4">
                {t("resultTemplate")} / {t("rejectionTemplate")} / {t("requestInfoTemplate")}
              </th>
              <th />
            </tr>
          </thead>
          <tbody>
            {rows.map((rt) => (
              <TypeRow key={rt.id} client={client} rt={rt} canUpdate={canUpdate} />
            ))}
          </tbody>
        </table>
      </section>

      <section className="space-y-2 rounded-lg border border-slate-200 bg-white p-4">
        <h2 className="font-medium">{t("templatesTitle")}</h2>
        <p className="text-sm text-slate-600">{t("templatesIntro")}</p>
        {canUpdate && (
          <div className="grid gap-2 sm:grid-cols-[1fr_120px_2fr_auto] sm:items-end">
            <label className="block text-sm">
              {t("newTemplateCode")}
              <input value={code} onChange={(e) => setCode(e.target.value)} placeholder={t("newTemplateCodePlaceholder")} className={INPUT} data-testid="new-template-code" />
            </label>
            <label className="block text-sm">
              {t("language")}
              <select value={language} onChange={(e) => setLanguage(e.target.value as "th" | "en")} className={INPUT} data-testid="new-template-lang">
                <option value="th">TH</option>
                <option value="en">EN</option>
              </select>
            </label>
            <label className="block text-sm">
              {t("content")}
              <input value={text} onChange={(e) => setText(e.target.value)} className={INPUT} data-testid="new-template-content" />
            </label>
            <button type="button" onClick={onSaveTemplate} disabled={saveTemplate.isPending} className="rounded-md bg-blue-700 px-3 py-1.5 text-sm text-white disabled:opacity-60" data-testid="save-template">
              {t("saveTemplate")}
            </button>
          </div>
        )}
        {tplError && <p className="text-sm text-rose-700" role="alert">{tplError}</p>}

        <table className="w-full text-sm">
          <thead className="text-left text-slate-500">
            <tr>
              <th className="py-1">{t("code")}</th>
              <th>{t("language")}</th>
            </tr>
          </thead>
          <tbody>
            {overrides.map((tpl) => (
              <tr key={tpl.id} className="border-t border-slate-100" data-testid={`template-row-${tpl.code}-${tpl.language}`}>
                <td className="py-1.5">{tpl.code}</td>
                <td>{tpl.language}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {overrides.length === 0 && <p className="text-sm text-slate-500">{t("noTemplates")}</p>}
      </section>
    </main>
  );
}
