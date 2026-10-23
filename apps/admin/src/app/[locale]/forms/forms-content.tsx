"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@pdpa/ui";
import { createApiClient, useFormMutations, useForms, useFormTypes, useMyFormSections } from "@pdpa/api-client";
import { text, type Language } from "@pdpa/form-renderer";
import { Link, useRouter } from "@/i18n/routing";
import { useLocale } from "next-intl";
import { problemCode } from "@/components/form-messages";

export function FormsContent() {
  const t = useTranslations("forms");
  const lang = useLocale() as Language;
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const types = useFormTypes(client);
  const forms = useForms(client);
  const mine = useMyFormSections(client);
  const creatable = (types.data ?? []).filter((x) => x.can_create);

  return (
    <main className="mx-auto max-w-6xl space-y-8 p-8 text-sm">
      <h1 className="text-xl font-semibold">{t("title")}</h1>
      {mine.data && mine.data.length > 0 && (
        <section className="space-y-2" aria-labelledby="my-sections">
          <h2 id="my-sections" className="text-base font-semibold">{t("mySections")}</h2>
          <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
            {mine.data.map((it) => (
              <li key={`${it.response_id}-${it.section}`} className="px-3 py-2">
                <Link href={`/form-responses/${it.response_id}`} className="font-medium underline">
                  {it.form_name} — {text(it.section_title, lang)}
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}
      <section className="space-y-2">
        {types.isPending || forms.isPending ? (
          <p className="text-slate-500">{t("loading")}</p>
        ) : forms.isError ? (
          <p className="text-red-700">{t("loadError")}</p>
        ) : forms.data.length === 0 ? (
          <p className="text-slate-500">{t("empty")}</p>
        ) : (
          <table className="w-full rounded-md border border-slate-200 bg-white">
            <thead className="bg-slate-50 text-left text-slate-600">
              <tr>
                <th className="px-3 py-2">{t("name")}</th>
                <th className="px-3 py-2">{t("type")}</th>
                <th className="px-3 py-2">{t("statusLabel")}</th>
                <th className="px-3 py-2">{t("version")}</th>
              </tr>
            </thead>
            <tbody>
              {forms.data.map((f) => (
                <tr key={f.id} className="border-t border-slate-100">
                  <td className="px-3 py-2">
                    <Link href={`/forms/${f.id}`} className="font-medium underline">{f.name}</Link>
                    <span className="ml-2 text-xs text-slate-500">{f.code}</span>
                  </td>
                  <td className="px-3 py-2">{t(`types.${f.form_type}`)}</td>
                  <td className="px-3 py-2">{t(`status.${f.status}`)}</td>
                  <td className="px-3 py-2">{f.latest_version}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
      {creatable.length > 0 && <CreateForm client={client} types={creatable.map((x) => x.form_type)} />}
    </main>
  );
}

function CreateForm({ client, types }: { client: ReturnType<typeof createApiClient>; types: string[] }) {
  const t = useTranslations("forms");
  const router = useRouter();
  const m = useFormMutations(client);
  const [formType, setFormType] = useState(types[0]!);
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const create = () =>
    m.create.mutate(
      {
        code,
        name,
        form_type: formType,
        draft: {
          schema: { sections: [{ key: "section_1", title: { th: t("builder.newSection") }, questions: [{ key: "question_1", type: "text", label: { th: t("builder.newQuestion") } }] }] },
        },
      },
      { onSuccess: (f) => router.push(`/forms/${f.id}`) },
    );
  const err = m.create.error ? (problemCode(m.create.error) === "forms.invalid_request" ? t("create.invalid") : t("actionError")) : null;
  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4" aria-labelledby="create-form">
      <h2 id="create-form" className="text-base font-semibold">{t("create.title")}</h2>
      <div className="grid gap-3 sm:grid-cols-3">
        <label className="block">
          <span className="block text-slate-600">{t("type")}</span>
          <select className="mt-1 w-full rounded border border-slate-300 px-2 py-1" value={formType} onChange={(e) => setFormType(e.target.value)}>
            {types.map((x) => <option key={x} value={x}>{t(`types.${x}`)}</option>)}
          </select>
        </label>
        <div>
          <label className="block">
            <span className="block text-slate-600">{t("code")}</span>
            <input className="mt-1 w-full rounded border border-slate-300 px-2 py-1 font-mono" value={code} onChange={(e) => setCode(e.target.value)} pattern="[a-z][a-z0-9_]*" aria-describedby="code-help" />
          </label>
          <p id="code-help" className="text-xs text-slate-500">{t("create.codeHelp")}</p>
        </div>
        <label className="block">
          <span className="block text-slate-600">{t("name")}</span>
          <input className="mt-1 w-full rounded border border-slate-300 px-2 py-1" value={name} onChange={(e) => setName(e.target.value)} />
        </label>
      </div>
      <Button onClick={create} disabled={m.create.isPending || !code || !name.trim()}>{t("create.submit")}</Button>
      {err && <p role="alert" className="text-red-700">{err}</p>}
    </section>
  );
}
