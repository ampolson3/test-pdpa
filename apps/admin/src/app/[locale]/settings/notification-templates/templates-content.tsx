"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useDeleteNotificationTemplate,
  useNotificationTemplates,
  useSaveNotificationTemplate,
  useTemplatePreview,
  type NotificationChannel,
  type NotificationTemplate,
} from "@pdpa/api-client";

const CHANNELS: NotificationChannel[] = ["email", "sms", "line", "in_app"];

type Draft = { id?: string; rowVersion?: number; global?: boolean; code: string; channel: NotificationChannel; language: "th" | "en"; subject: string; body: string; variables: string };

const empty: Draft = { code: "", channel: "email", language: "th", subject: "", body: "", variables: "" };

function fromTemplate(t: NotificationTemplate): Draft {
  return {
    id: t.global ? undefined : t.id,
    rowVersion: t.row_version,
    global: t.global,
    code: t.code,
    channel: t.channel,
    language: t.language === "en" ? "en" : "th",
    subject: t.subject ?? "",
    body: t.body,
    variables: t.variables.join(", "),
  };
}

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const h = setTimeout(() => setV(value), ms);
    return () => clearTimeout(h);
  }, [value, ms]);
  return v;
}

export function TemplatesContent() {
  const t = useTranslations("notify");
  const canRead = usePermission("admin.notification.read");
  const canCreate = usePermission("admin.notification.create");
  const canUpdate = usePermission("admin.notification.update");
  const canDelete = usePermission("admin.notification.delete");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const list = useNotificationTemplates(client);
  const save = useSaveNotificationTemplate(client);
  const remove = useDeleteNotificationTemplate(client);
  const [draft, setDraft] = useState<Draft | null>(null);

  const variables = (draft?.variables ?? "").split(",").map((v) => v.trim()).filter(Boolean);
  const preview = useTemplatePreview(client, useDebounced({ subject: draft?.subject || undefined, body: draft?.body ?? "", variables }, 400));

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  // Editing a global template creates the tenant's own override (same code, channel, language).
  const editable = draft && (draft.id ? canUpdate : canCreate);
  const submit = () => {
    if (!draft) return;
    save.mutate(
      { id: draft.id, rowVersion: draft.rowVersion, input: { code: draft.code, channel: draft.channel, language: draft.language, subject: draft.subject || undefined, body: draft.body, variables } },
      { onSuccess: (saved) => setDraft(fromTemplate(saved)) },
    );
  };

  return (
    <main className="mx-auto grid max-w-6xl gap-6 p-8 lg:grid-cols-[1fr_1.4fr]">
      <section className="space-y-3">
        <header className="flex items-center justify-between">
          <h1 className="text-xl font-semibold">{t("templates.title")}</h1>
          {canCreate && (
            <Button variant="secondary" onClick={() => setDraft({ ...empty })}>
              {t("templates.new")}
            </Button>
          )}
        </header>
        {list.isPending ? (
          <p className="text-slate-500">{t("loading")}</p>
        ) : list.isError ? (
          <p className="text-red-700">{t("loadError")}</p>
        ) : list.data.length === 0 ? (
          <p className="text-slate-500">{t("templates.empty")}</p>
        ) : (
          <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white text-sm">
            {list.data.map((tpl) => (
              <li key={tpl.id}>
                <button className="flex w-full items-center justify-between gap-2 px-3 py-2 text-left hover:bg-slate-50" onClick={() => setDraft(fromTemplate(tpl))}>
                  <span className="font-mono text-xs">{tpl.code}</span>
                  <span className="text-xs text-slate-500">
                    {t(`channel.${tpl.channel}`)} · {tpl.language.toUpperCase()} {tpl.global ? `· ${t("templates.global")}` : ""}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      {draft && (
        <section className="space-y-4">
          {draft.global && <p className="rounded-md bg-amber-50 p-3 text-sm text-amber-800">{t("templates.globalHint")}</p>}
          <div className="grid grid-cols-3 gap-3 text-sm">
            <label className="col-span-3 sm:col-span-1">
              <span className="block text-slate-600">{t("templates.code")}</span>
              <input className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1 font-mono" value={draft.code} disabled={!!draft.id || draft.global}
                onChange={(e) => setDraft({ ...draft, code: e.target.value })} placeholder="dsar.received" />
            </label>
            <label>
              <span className="block text-slate-600">{t("templates.channel")}</span>
              <select className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1" value={draft.channel} disabled={!!draft.id || draft.global}
                onChange={(e) => setDraft({ ...draft, channel: e.target.value as NotificationChannel })}>
                {CHANNELS.map((c) => (
                  <option key={c} value={c}>{t(`channel.${c}`)}</option>
                ))}
              </select>
            </label>
            <label>
              <span className="block text-slate-600">{t("templates.language")}</span>
              <select className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1" value={draft.language} disabled={!!draft.id || draft.global}
                onChange={(e) => setDraft({ ...draft, language: e.target.value as "th" | "en" })}>
                <option value="th">ไทย</option>
                <option value="en">English</option>
              </select>
            </label>
          </div>
          {(draft.channel === "email" || draft.channel === "in_app") && (
            <label className="block text-sm">
              <span className="block text-slate-600">{t("templates.subject")}</span>
              <input className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1" value={draft.subject} onChange={(e) => setDraft({ ...draft, subject: e.target.value })} />
            </label>
          )}
          <label className="block text-sm">
            <span className="block text-slate-600">{t("templates.body")}</span>
            <textarea className="mt-1 h-40 w-full rounded-md border border-slate-300 px-2 py-1 font-mono text-xs" value={draft.body} onChange={(e) => setDraft({ ...draft, body: e.target.value })} />
          </label>
          <label className="block text-sm">
            <span className="block text-slate-600">{t("templates.variables")}</span>
            <input className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1 font-mono text-xs" value={draft.variables} placeholder="name, request_no"
              onChange={(e) => setDraft({ ...draft, variables: e.target.value })} />
            <span className="mt-1 block text-xs text-slate-500">{t("templates.variablesHint")}</span>
          </label>

          <div className="rounded-md border border-slate-200 bg-slate-50 p-3 text-sm">
            <h2 className="mb-2 text-xs font-medium uppercase text-slate-500">{t("templates.preview")}</h2>
            {preview.isError ? (
              <p className="text-red-700">{t("templates.invalid")}</p>
            ) : preview.data ? (
              <>
                {preview.data.subject && <p className="font-medium">{preview.data.subject}</p>}
                <p className="whitespace-pre-wrap">{preview.data.body}</p>
              </>
            ) : null}
          </div>

          {save.isError && <p className="text-sm text-red-700">{t("templates.saveError")}</p>}
          <div className="flex gap-2">
            {editable && (
              <Button onClick={submit} disabled={save.isPending || !draft.body.trim() || (!draft.id && !draft.code)}>
                {draft.global ? t("templates.override") : t("save")}
              </Button>
            )}
            {draft.id && canDelete && (
              <Button variant="ghost" onClick={() => remove.mutate({ id: draft.id!, rowVersion: draft.rowVersion! }, { onSuccess: () => setDraft(null) })}>
                {t("templates.delete")}
              </Button>
            )}
            <Button variant="ghost" onClick={() => setDraft(null)}>{t("cancel")}</Button>
          </div>
        </section>
      )}
    </main>
  );
}
