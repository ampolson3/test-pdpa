"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { createApiClient, useKbArticles, useKbArticleMutations, type DpoKbArticle, type DpoKbArticleInput } from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const SOURCES = ["law", "pdpc", "policy", "faq", "internal"] as const;

type Draft = { id?: string; rowVersion?: number; category: string; title: string; text: string; tags: string; source: (typeof SOURCES)[number]; language: "th" | "en" };
const blank: Draft = { category: "", title: "", text: "", tags: "", source: "policy", language: "th" };

function bodyText(a: DpoKbArticle): string {
  const b = (a.body ?? {}) as Record<string, unknown>;
  if (typeof b.text === "string") return b.text;
  return [b.question, b.answer].filter((v) => typeof v === "string").join("\n\n");
}

function detail(e: unknown): string {
  return typeof e === "object" && e !== null
    ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ")
    : "";
}

export function KnowledgeBaseContent() {
  const t = useTranslations("knowledgeBase");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const canRead = usePermission("dpo.kb.read");
  const canCreate = usePermission("dpo.kb.create");
  const canUpdate = usePermission("dpo.kb.update");
  const canPublish = usePermission("dpo.kb.publish");
  const canArchive = usePermission("dpo.kb.delete");
  const [q, setQ] = useState("");
  const [source, setSource] = useState("");
  const [showAll, setShowAll] = useState(false);
  const [open, setOpen] = useState<string | undefined>();
  const [draft, setDraft] = useState<Draft | undefined>();
  const articles = useKbArticles(client, {
    q: q || undefined,
    source: (source || undefined) as DpoKbArticleInput["source"] | undefined,
    include_all: canUpdate && showAll ? true : undefined,
  });
  const { save, setStatus } = useKbArticleMutations(client);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const submit = () => {
    if (!draft) return;
    save.mutate(
      {
        id: draft.id,
        rowVersion: draft.rowVersion,
        body: {
          category: draft.category,
          title: draft.title,
          source: draft.source,
          language: draft.language,
          body: { text: draft.text },
          tags: draft.tags.split(",").map((s) => s.trim()).filter(Boolean),
        },
      },
      { onSuccess: () => setDraft(undefined) },
    );
  };

  return (
    <main className="mx-auto max-w-5xl space-y-6 p-8">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">{t("title")}</h1>
          <p className="text-sm text-slate-600">{t("intro")}</p>
        </div>
        {canCreate && !draft && <Button onClick={() => setDraft(blank)}>{t("add")}</Button>}
      </header>

      <section className="flex flex-wrap items-end gap-3">
        <label className="grow text-sm">
          {t("search")}
          <input className={INPUT} data-testid="kb-search" value={q} placeholder={t("searchPlaceholder")} onChange={(e) => setQ(e.target.value)} />
        </label>
        <label className="text-sm">
          {t("source")}
          <select className={INPUT} value={source} onChange={(e) => setSource(e.target.value)}>
            <option value="">{t("allSources")}</option>
            {SOURCES.map((s) => (
              <option key={s} value={s}>
                {t(`sources.${s}`)}
              </option>
            ))}
          </select>
        </label>
        {canUpdate && (
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={showAll} onChange={(e) => setShowAll(e.target.checked)} />
            {t("showAll")}
          </label>
        )}
      </section>

      {draft && (
        <section className="space-y-3 rounded-md border border-slate-200 p-4">
          <label className="block text-sm">
            {t("fields.title")}
            <input className={INPUT} value={draft.title} onChange={(e) => setDraft({ ...draft, title: e.target.value })} />
          </label>
          <div className="grid grid-cols-3 gap-3">
            <label className="text-sm">
              {t("fields.category")}
              <input className={INPUT} value={draft.category} onChange={(e) => setDraft({ ...draft, category: e.target.value })} />
            </label>
            <label className="text-sm">
              {t("source")}
              <select className={INPUT} value={draft.source} onChange={(e) => setDraft({ ...draft, source: e.target.value as Draft["source"] })}>
                {SOURCES.map((s) => (
                  <option key={s} value={s}>
                    {t(`sources.${s}`)}
                  </option>
                ))}
              </select>
            </label>
            <label className="text-sm">
              {t("fields.language")}
              <select className={INPUT} value={draft.language} onChange={(e) => setDraft({ ...draft, language: e.target.value as "th" | "en" })}>
                <option value="th">ไทย</option>
                <option value="en">English</option>
              </select>
            </label>
          </div>
          <label className="block text-sm">
            {t("fields.body")}
            <textarea className={INPUT} rows={6} value={draft.text} onChange={(e) => setDraft({ ...draft, text: e.target.value })} />
          </label>
          <label className="block text-sm">
            {t("fields.tags")}
            <input className={INPUT} value={draft.tags} onChange={(e) => setDraft({ ...draft, tags: e.target.value })} />
          </label>
          <div className="flex gap-2">
            <Button disabled={save.isPending || !draft.title || !draft.category || !draft.text} onClick={submit}>
              {t("save")}
            </Button>
            <button className="text-sm underline" onClick={() => setDraft(undefined)}>
              {t("cancel")}
            </button>
          </div>
          {save.isError && <p className="text-sm text-red-700">{t("error", { detail: detail(save.error) })}</p>}
        </section>
      )}

      <ul className="space-y-2">
        {(articles.data ?? []).map((a) => (
          <li key={a.id} className="rounded-md border border-slate-200 p-3">
            <button className="w-full text-left" onClick={() => setOpen(open === a.id ? undefined : a.id)}>
              <span className="font-medium">{a.title}</span>
              <span className="ml-2 rounded bg-slate-100 px-2 py-0.5 text-xs">{t(`sources.${a.source}`)}</span>
              <span className="ml-2 text-xs text-slate-500">{a.category} · v{a.version_no ?? 1}</span>
              {a.status && a.status !== "published" && (
                <span className="ml-2 rounded bg-amber-100 px-2 py-0.5 text-xs">{t(`statuses.${a.status}`)}</span>
              )}
              {a.is_global && <span className="ml-2 rounded bg-blue-100 px-2 py-0.5 text-xs">{t("global")}</span>}
            </button>
            {open === a.id && (
              <div className="mt-2 space-y-2">
                <p className="whitespace-pre-wrap text-sm text-slate-700">{bodyText(a)}</p>
                {a.tags.length > 0 && <p className="text-xs text-slate-500">{a.tags.join(" · ")}</p>}
                {!a.is_global && (
                  <div className="flex gap-3 text-sm">
                    {canUpdate && a.status !== "archived" && (
                      <button
                        className="underline"
                        onClick={() =>
                          setDraft({
                            id: a.id,
                            rowVersion: a.row_version,
                            category: a.category,
                            title: a.title,
                            text: bodyText(a),
                            tags: a.tags.join(", "),
                            source: a.source,
                            language: a.language === "en" ? "en" : "th",
                          })
                        }
                      >
                        {t("edit")}
                      </button>
                    )}
                    {canPublish && a.status !== "published" && (
                      <button className="text-green-700 underline" onClick={() => setStatus.mutate({ id: a.id, rowVersion: a.row_version, status: "published" })}>
                        {t("publish")}
                      </button>
                    )}
                    {canArchive && a.status !== "archived" && (
                      <button className="text-red-700 underline" onClick={() => setStatus.mutate({ id: a.id, rowVersion: a.row_version, status: "archived" })}>
                        {t("archive")}
                      </button>
                    )}
                  </div>
                )}
              </div>
            )}
          </li>
        ))}
      </ul>
      {articles.data?.length === 0 && <p className="text-sm text-slate-500">{t("empty")}</p>}
    </main>
  );
}
