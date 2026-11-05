"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@pdpa/ui";
import { createApiClient, useFormResponse, useFormResponseMutations, useMentionSearch, type FormResponse } from "@pdpa/api-client";
import { FormRenderer, text, type FieldError, type FormSchema, type Language, type Scoring, type Section } from "@pdpa/form-renderer";
import { Link } from "@/i18n/routing";
import { answerErrors, problemCode, useRendererMessages } from "@/components/form-messages";

type Client = ReturnType<typeof createApiClient>;

export function ResponseContent({ id }: { id: string }) {
  const t = useTranslations("forms");
  const locale = useLocale() as Language;
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const resp = useFormResponse(client, id);
  const m = useFormResponseMutations(client);
  const [lang, setLang] = useState<Language>(locale);
  const messages = useRendererMessages(lang);
  const [serverErrors, setServerErrors] = useState<FieldError[]>();
  const [notice, setNotice] = useState<string>();
  const [error, setError] = useState<string>();

  if (resp.isPending) return <main className="p-8 text-slate-500">{t("loading")}</main>;
  if (resp.isError) return <main className="p-8 text-red-700">{t("loadError")}</main>;
  const r = resp.data;
  const schema = r.version.schema as FormSchema;
  const scoring = (r.version.scoring ?? null) as Scoring | null;
  const submitted = r.status === "submitted";
  const english = r.version.languages.includes("en");

  const fail = (e: unknown) => {
    const fields = answerErrors(e);
    setServerErrors(fields);
    const code = problemCode(e);
    setError(fields ? t("response.fixAnswers") : code === "forms.invalid_state" ? t("response.sectionsOpen") : code === "conflict.version_mismatch" ? t("response.conflict") : t("actionError"));
  };
  const ok = (msg: string) => {
    setServerErrors(undefined);
    setError(undefined);
    setNotice(msg);
  };
  const save = async (answers: Record<string, unknown>): Promise<FormResponse | undefined> => {
    try {
      return await m.saveAnswers.mutateAsync({ response: r, answers });
    } catch (e) {
      fail(e);
      return undefined;
    }
  };
  const band = r.result.band ? text(scoring?.bands.find((b) => b.key === r.result.band)?.label, lang) || r.result.band : undefined;

  return (
    <main className="mx-auto max-w-4xl space-y-4 p-8 text-sm">
      <header className="space-y-1">
        <Link href={`/forms/${r.form.id}`} className="text-slate-500 underline">{r.form.name}</Link>
        <h1 className="text-xl font-semibold">{t("response.title", { name: r.form.name, no: r.version.version })}</h1>
        <p className="text-slate-600">
          {t(`responses.status.${r.status}`)}
          {r.owner_name && <> · {t("response.owner", { name: r.owner_name })}</>}
        </p>
        {english && (
          <div role="group" aria-label={t("builder.previewLanguage")} className="flex gap-1">
            {(["th", "en"] as const).map((l) => (
              <button key={l} aria-pressed={lang === l} className={`rounded px-2 py-0.5 ${lang === l ? "bg-slate-900 text-white" : "bg-slate-100"}`} onClick={() => setLang(l)}>{t(`builder.${l}`)}</button>
            ))}
          </div>
        )}
      </header>
      {submitted && (
        <p role="status" className="rounded-md border border-emerald-200 bg-emerald-50 p-3 text-emerald-900" data-testid="result">
          {t("response.result", { score: r.result.score, max: r.result.max_score })}
          {band && <> · {t("response.band", { band })}</>}
        </p>
      )}
      {notice && !submitted && <p role="status" className="text-emerald-800">{notice}</p>}
      {error && <p role="alert" className="text-red-700">{error}</p>}
      <FormRenderer
        schema={schema}
        scoring={scoring}
        language={lang}
        messages={messages}
        values={r.answers}
        editable={r.can_answer}
        disabled={submitted}
        serverErrors={serverErrors}
        showScore={!!scoring && !submitted}
        sectionExtra={(sec) => <SectionBar r={r} section={sec} client={client} onError={fail} />}
        onSaveDraft={async (answers) => {
          if (await save(answers)) ok(t("response.saved"));
        }}
        onSubmit={async (answers) => {
          let current = await save(answers);
          if (!current) return;
          try {
            if (r.is_owner) {
              await m.submit.mutateAsync({ response: current });
              ok(t("response.submitted"));
              return;
            }
            // An assignee hands back every section they were given.
            for (const section of current.can_answer) current = await m.completeSection.mutateAsync({ response: current, section });
            ok(t("response.sectionDone"));
          } catch (e) {
            fail(e);
          }
        }}
        actions={({ submit, saveDraft, busy }) =>
          submitted || r.can_answer.length === 0 ? null : (
            <div className="flex gap-2">
              <Button type="button" variant="secondary" onClick={saveDraft} disabled={busy}>{t("response.saveDraft")}</Button>
              <Button type="button" onClick={submit} disabled={busy}>{r.is_owner ? t("response.submit") : t("response.completeMine")}</Button>
            </div>
          )
        }
      />
    </main>
  );
}

function SectionBar({ r, section, client, onError }: { r: FormResponse; section: Section; client: Client; onError: (e: unknown) => void }) {
  const t = useTranslations("forms.response");
  const m = useFormResponseMutations(client);
  const a = r.assignments.find((x) => x.section === section.key);
  const [picking, setPicking] = useState(false);
  const [q, setQ] = useState("");
  const users = useMentionSearch(client, picking && q.trim() ? q.trim() : null);
  const draft = r.status === "draft";

  const assign = (userId: string | null) =>
    m.assign.mutate({ response: r, section: section.key, userId }, { onSuccess: () => { setPicking(false); setQ(""); }, onError });
  return (
    <div className="mb-3 flex flex-wrap items-center gap-2 text-xs" data-testid={`section-bar-${section.key}`}>
      {a ? (
        <span className="rounded bg-slate-100 px-2 py-0.5">
          {a.status === "done" ? t("assignedDone", { name: a.assignee_name }) : t("assignedTo", { name: a.assignee_name })}
        </span>
      ) : null}
      {r.is_owner && draft && (
        <>
          {a && a.status === "open" && <button className="underline" onClick={() => assign(null)}>{t("unassign")}</button>}
          {!picking ? (
            <button className="underline" onClick={() => setPicking(true)}>{a ? t("reassign") : t("assign")}</button>
          ) : (
            <span className="relative">
              <input autoFocus aria-label={t("searchUser")} placeholder={t("searchUser")} className="rounded border border-slate-300 px-2 py-0.5" value={q} onChange={(e) => setQ(e.target.value)} />
              {users.data && users.data.length > 0 && (
                <ul className="absolute z-10 mt-1 w-56 rounded border border-slate-200 bg-white shadow">
                  {users.data.map((u) => (
                    <li key={u.id}><button className="w-full px-2 py-1 text-left hover:bg-slate-50" onClick={() => assign(u.id)}>{u.display_name}</button></li>
                  ))}
                </ul>
              )}
              <button className="ml-1 underline" onClick={() => setPicking(false)}>{t("cancel")}</button>
            </span>
          )}
        </>
      )}
    </div>
  );
}
