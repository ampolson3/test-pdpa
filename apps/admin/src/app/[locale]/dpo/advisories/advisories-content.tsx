"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useAdvisories,
  useSubmitAdvisory,
  useAnswerAdvisory,
  useKbArticles,
  type DpoAdvisory,
} from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

const STATUS_STYLE: Record<string, string> = {
  open: "bg-sky-100 text-sky-800",
  answered: "bg-emerald-100 text-emerald-800",
  closed: "bg-slate-100 text-slate-500",
};

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

/** DPO-07: a department sends a question to the DPO, tracked until answered; answering a question also
 * promotes it into the searchable knowledge base in the same step — the acceptance criterion itself
 * ("คำถามที่ตอบแล้วค้นได้ในคลังคำตอบ"), so this page's own KB search panel shows it immediately. */
export function AdvisoriesContent() {
  const t = useTranslations("dpoAdvisories");
  const canRead = usePermission("dpo.advisory.read");
  const canCreate = usePermission("dpo.advisory.create");
  const canAnswer = usePermission("dpo.advisory.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);

  const [status, setStatus] = useState<string>("");
  const list = useAdvisories(client, { status: status || undefined });
  const submit = useSubmitAdvisory(client);
  const answerMut = useAnswerAdvisory(client);

  const [asking, setAsking] = useState(false);
  const [subject, setSubject] = useState("");
  const [question, setQuestion] = useState("");
  const [answering, setAnswering] = useState<DpoAdvisory | null>(null);
  const [answer, setAnswer] = useState("");

  const [kbQuery, setKbQuery] = useState("");
  const kb = useKbArticles(client, { q: kbQuery.length >= 2 ? kbQuery : undefined });

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data ?? [];

  const submitAsk = () => {
    if (!subject.trim() || !question.trim()) return;
    submit.mutate({ subject: subject.trim(), question: question.trim() }, { onSuccess: () => { setAsking(false); setSubject(""); setQuestion(""); } });
  };

  const submitAnswer = () => {
    if (!answering || !answer.trim()) return;
    answerMut.mutate({ advisory: answering, answer: answer.trim() }, { onSuccess: () => { setAnswering(null); setAnswer(""); } });
  };

  return (
    <main className="mx-auto max-w-5xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        {canCreate && <Button onClick={() => setAsking(true)} data-testid="ask-dpo">{t("ask")}</Button>}
      </header>

      <section className="rounded-md border border-slate-200 bg-white p-4" data-testid="kb-search">
        <h2 className="font-semibold">{t("kb.title")}</h2>
        <input className={INPUT} value={kbQuery} onChange={(e) => setKbQuery(e.target.value)} placeholder={t("kb.searchPlaceholder")} />
        {kbQuery.length >= 2 && (
          <ul className="mt-2 divide-y divide-slate-100">
            {(kb.data ?? []).length === 0 ? (
              <li className="py-2 text-slate-500">{t("kb.empty")}</li>
            ) : (
              kb.data!.map((a) => (
                <li key={a.id} className="py-2">
                  <p className="font-medium">{a.title}</p>
                  {typeof a.body?.answer === "string" && <p className="text-slate-600">{a.body.answer}</p>}
                </li>
              ))
            )}
          </ul>
        )}
      </section>

      <label className="block w-48"><span className="block text-slate-600">{t("filters.status")}</span>
        <select className={INPUT} value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">{t("filters.all")}</option>
          {["open", "answered", "closed"].map((s) => <option key={s} value={s}>{t(`statuses.${s}`)}</option>)}
        </select>
      </label>

      {asking && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4" data-testid="ask-form">
          <legend className="px-1 font-semibold">{t("form.askTitle")}</legend>
          <label><span className="block text-slate-600">{t("form.subject")}</span>
            <input className={INPUT} value={subject} onChange={(e) => setSubject(e.target.value)} /></label>
          <label><span className="block text-slate-600">{t("form.question")}</span>
            <textarea className={INPUT} value={question} onChange={(e) => setQuestion(e.target.value)} /></label>
          {submit.isError && <p className="text-red-700">{detail(submit.error)}</p>}
          <div className="flex gap-2">
            <Button onClick={submitAsk} disabled={!subject.trim() || !question.trim() || submit.isPending}>{t("form.submit")}</Button>
            <Button onClick={() => setAsking(false)} className="bg-slate-200 text-slate-700 hover:bg-slate-300">{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      {answering && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4" data-testid="answer-form">
          <legend className="px-1 font-semibold">{t("form.answerTitle")}</legend>
          <p className="text-slate-700">{answering.question}</p>
          <label><span className="block text-slate-600">{t("form.answer")}</span>
            <textarea className={INPUT} value={answer} onChange={(e) => setAnswer(e.target.value)} /></label>
          {answerMut.isError && <p className="text-red-700">{detail(answerMut.error)}</p>}
          <div className="flex gap-2">
            <Button onClick={submitAnswer} disabled={!answer.trim() || answerMut.isPending}>{t("form.submit")}</Button>
            <Button onClick={() => setAnswering(null)} className="bg-slate-200 text-slate-700 hover:bg-slate-300">{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      {list.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : rows.length === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : (
        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="border-b border-slate-200 text-slate-600">
              <th className="py-2">{t("table.subject")}</th>
              <th className="py-2">{t("table.requester")}</th>
              <th className="py-2">{t("table.status")}</th>
              <th className="py-2"></th>
            </tr>
          </thead>
          <tbody>
            {rows.map((a) => (
              <tr key={a.id} className="border-b border-slate-100">
                <td className="py-2">{a.subject}</td>
                <td className="py-2 text-slate-600">{a.requester_name}</td>
                <td className="py-2"><span className={`rounded px-2 py-0.5 text-xs ${STATUS_STYLE[a.status] ?? ""}`}>{t(`statuses.${a.status}`)}</span></td>
                <td className="py-2 text-right">
                  {canAnswer && a.status === "open" && (
                    <Button onClick={() => setAnswering(a)} data-testid={`answer-${a.id}`}>{t("answer")}</Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
