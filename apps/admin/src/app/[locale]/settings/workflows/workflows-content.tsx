"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useCalendars,
  useGroupSearch,
  useMentionSearch,
  useSaveWorkflowDefinition,
  useWorkflowDefinitions,
  type WorkflowDefinition,
  type WorkflowDefinitionInput,
} from "@pdpa/api-client";


type Mode = "calendar_days" | "business_days" | "hours";
type Ref = { id: string; name: string };
type StepDraft = { key: string; th: string; en: string; terminal: boolean; pause: boolean; taskKind: "none" | "user" | "group"; taskTitle: string; assignee?: Ref };
type Draft = {
  id?: string; rowVersion?: number; version?: number; global?: boolean;
  code: string; name: string; entityType: string; initial: string; steps: StepDraft[]; moves: string[];
  slaOn: boolean; slaCode: string; mode: Mode; amount: number; remind: string; calendarId: string; escalate: Ref[];
};

const INPUT = "mt-1 w-full rounded-md border border-slate-300 px-2 py-1";
const MODES: Mode[] = ["calendar_days", "business_days", "hours"];
const move = (from: string, to: string) => `${from}>${to}`;

const blank: Draft = {
  code: "", name: "", entityType: "", initial: "received",
  steps: [
    { key: "received", th: "รับเรื่อง", en: "Received", terminal: false, pause: false, taskKind: "none", taskTitle: "" },
    { key: "done", th: "เสร็จสิ้น", en: "Done", terminal: true, pause: false, taskKind: "none", taskTitle: "" },
  ],
  moves: [move("received", "done")],
  slaOn: true, slaCode: "response", mode: "calendar_days", amount: 30, remind: "10, 5", calendarId: "", escalate: [],
};

function fromDefinition(d: WorkflowDefinition): Draft {
  const names = d.names ?? {};
  const ref = (id?: string): Ref | undefined => (id ? { id, name: names[id] ?? id } : undefined);
  const sla = d.definition.sla;
  return {
    id: d.id, rowVersion: d.row_version, version: d.version, global: d.global,
    code: d.code, name: d.name, entityType: d.entity_type, initial: d.definition.initial,
    steps: d.definition.states.map((s) => ({
      key: s.key, th: s.label.th, en: s.label.en ?? "", terminal: !!s.terminal, pause: !!s.pause_sla,
      taskKind: s.task ? (s.task.assignee_group_id ? "group" : "user") : "none", taskTitle: s.task?.title.th ?? "",
      assignee: ref(s.task?.assignee_group_id ?? s.task?.assignee_user_id),
    })),
    moves: d.definition.transitions.map((t) => move(t.from, t.to)),
    slaOn: !!sla, slaCode: sla?.code ?? "response", mode: (sla?.mode as Mode) ?? "calendar_days", amount: sla?.amount ?? 30,
    remind: (sla?.remind_before ?? []).join(", "), calendarId: sla?.calendar_id ?? "",
    escalate: (sla?.escalate_user_ids ?? []).map((id) => ({ id, name: names[id] ?? id })),
  };
}

function toInput(d: Draft): WorkflowDefinitionInput {
  const keys = new Set(d.steps.map((s) => s.key));
  return {
    code: d.code, name: d.name, entity_type: d.entityType,
    definition: {
      initial: d.initial,
      states: d.steps.map((s) => ({
        key: s.key, label: s.en ? { th: s.th, en: s.en } : { th: s.th }, terminal: s.terminal || undefined, pause_sla: s.pause || undefined,
        task: s.taskKind === "none" || !s.assignee ? undefined : {
          title: { th: s.taskTitle || s.th },
          ...(s.taskKind === "user" ? { assignee_user_id: s.assignee.id } : { assignee_group_id: s.assignee.id }),
        },
      })),
      transitions: d.moves.map((m) => m.split(">")).filter(([f, t]) => keys.has(f!) && keys.has(t!)).map(([from, to]) => ({ from: from!, to: to! })),
      sla: d.slaOn ? {
        code: d.slaCode, mode: d.mode, amount: d.amount,
        remind_before: d.remind.split(",").map((x) => Number(x.trim())).filter((n) => Number.isInteger(n) && n > 0).sort((a, b) => b - a),
        calendar_id: d.calendarId || undefined,
        escalate_user_ids: d.escalate.length ? d.escalate.map((r) => r.id) : undefined,
      } : undefined,
    },
  };
}

function problemText(e: unknown): string {
  if (typeof e === "object" && e !== null) {
    const p = e as { title?: string; detail?: string };
    return [p.title, p.detail].filter(Boolean).join(" — ");
  }
  return "";
}

export function WorkflowsContent() {
  const t = useTranslations("workflow");
  const canRead = usePermission("admin.workflow.read");
  const canCreate = usePermission("admin.workflow.create");
  const canUpdate = usePermission("admin.workflow.update");
  const canCalendars = usePermission("org.settings.read");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const list = useWorkflowDefinitions(client);
  const calendars = useCalendars(client);
  const save = useSaveWorkflowDefinition(client);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [savedVersion, setSavedVersion] = useState<number>();

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;
  const editable = draft && (draft.id ? canUpdate : canCreate);
  const set = (patch: Partial<Draft>) => { setSavedVersion(undefined); setDraft(draft && { ...draft, ...patch }); };
  const setStep = (i: number, patch: Partial<StepDraft>) => draft && set({ steps: draft.steps.map((s, j) => (j === i ? { ...s, ...patch } : s)) });

  const submit = () => {
    if (!draft) return;
    save.mutate({ id: draft.id, rowVersion: draft.rowVersion, input: toInput(draft) }, {
      onSuccess: (d) => { setDraft(fromDefinition(d)); setSavedVersion(d.version); },
    });
  };

  return (
    <main className="mx-auto grid max-w-7xl gap-6 p-8 lg:grid-cols-[1fr_3fr]">
      <section className="space-y-3">
        <header className="flex items-center justify-between">
          <h1 className="text-xl font-semibold">{t("editor.title")}</h1>
          {canCreate && <Button variant="secondary" onClick={() => { save.reset(); setSavedVersion(undefined); setDraft({ ...blank }); }}>{t("editor.new")}</Button>}
        </header>
        <p className="text-sm text-slate-600">{t("editor.intro")}</p>
        {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : list.data.length === 0 ? (
          <p className="text-sm text-slate-500">{t("editor.empty")}</p>
        ) : (
          <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white text-sm">
            {list.data.map((d) => (
              <li key={d.id}>
                <button className="flex w-full items-center justify-between gap-2 px-3 py-2 text-left hover:bg-slate-50" onClick={() => { save.reset(); setSavedVersion(undefined); setDraft(fromDefinition(d)); }}>
                  <span>{d.name}<span className="block font-mono text-xs text-slate-500">{d.code}</span></span>
                  <span className="text-xs text-slate-500">{d.global ? t("editor.global") : t("editor.version", { version: d.version })}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      {draft && (
        <fieldset className="space-y-6 text-sm" disabled={!editable}>
          <div className="grid gap-3 sm:grid-cols-3">
            <Field label={t("editor.code")}><input className={`${INPUT} font-mono`} value={draft.code} disabled={!!draft.id} onChange={(e) => set({ code: e.target.value })} placeholder="dsar_access" /></Field>
            <Field label={t("editor.name")}><input className={`${INPUT}`} value={draft.name} onChange={(e) => set({ name: e.target.value })} /></Field>
            <Field label={t("editor.entityType")}><input className={`${INPUT} font-mono`} value={draft.entityType} onChange={(e) => set({ entityType: e.target.value })} placeholder="dsar_request" /></Field>
          </div>

          <section className="space-y-2">
            <header className="flex items-center justify-between">
              <h2 className="font-semibold">{t("editor.states")}</h2>
              <Button variant="secondary" onClick={() => set({ steps: [...draft.steps, { key: `step_${draft.steps.length + 1}`, th: "", en: "", terminal: false, pause: false, taskKind: "none", taskTitle: "" }] })}>{t("editor.addState")}</Button>
            </header>
            {draft.steps.map((s, i) => (
              <div key={i} className="space-y-2 rounded-md border border-slate-200 bg-white p-3" data-testid={`step-${i}`}>
                <div className="grid gap-2 sm:grid-cols-4">
                  <Field label={t("editor.key")}><input className={`${INPUT} font-mono`} value={s.key} onChange={(e) => setStep(i, { key: e.target.value })} /></Field>
                  <Field label={t("editor.labelTh")}><input className={`${INPUT}`} value={s.th} onChange={(e) => setStep(i, { th: e.target.value })} /></Field>
                  <Field label={t("editor.labelEn")}><input className={`${INPUT}`} value={s.en} onChange={(e) => setStep(i, { en: e.target.value })} /></Field>
                  <div className="flex flex-wrap items-end gap-3 pb-1">
                    <label className="flex items-center gap-1"><input type="radio" name="initial" checked={draft.initial === s.key} onChange={() => set({ initial: s.key })} />{t("editor.initial")}</label>
                    <label className="flex items-center gap-1"><input type="checkbox" checked={s.terminal} onChange={(e) => setStep(i, { terminal: e.target.checked, taskKind: e.target.checked ? "none" : s.taskKind })} />{t("editor.terminal")}</label>
                    <label className="flex items-center gap-1"><input type="checkbox" checked={s.pause} disabled={s.terminal} onChange={(e) => setStep(i, { pause: e.target.checked })} />{t("editor.pauseSla")}</label>
                  </div>
                </div>
                {!s.terminal && (
                  <div className="grid gap-2 sm:grid-cols-4">
                    <Field label={t("editor.assignee")}>
                      <select className={`${INPUT} bg-white`} value={s.taskKind} onChange={(e) => setStep(i, { taskKind: e.target.value as StepDraft["taskKind"], assignee: undefined })}>
                        <option value="none">{t("editor.none")}</option>
                        <option value="user">{t("editor.user")}</option>
                        <option value="group">{t("editor.group")}</option>
                      </select>
                    </Field>
                    {s.taskKind !== "none" && (
                      <>
                        <div className="sm:col-span-2"><Picker kind={s.taskKind} value={s.assignee} onPick={(r) => setStep(i, { assignee: r })} /></div>
                        <Field label={t("editor.taskTitle")}><input className={`${INPUT}`} value={s.taskTitle} onChange={(e) => setStep(i, { taskTitle: e.target.value })} /></Field>
                      </>
                    )}
                  </div>
                )}
                {draft.steps.length > 2 && <button className="text-xs text-red-700 hover:underline" onClick={() => set({ steps: draft.steps.filter((_, j) => j !== i) })}>{t("editor.remove")}</button>}
              </div>
            ))}
          </section>

          <section className="space-y-2">
            <h2 className="font-semibold">{t("editor.transitions")}</h2>
            <p className="text-xs text-slate-500">{t("editor.transitionsHint")}</p>
            <div className="overflow-x-auto">
              <table className="rounded-md border border-slate-200 bg-white">
                <thead><tr><th />{draft.steps.map((s) => <th key={s.key} className="px-2 py-1 text-xs font-medium">{s.th || s.key}</th>)}</tr></thead>
                <tbody>
                  {draft.steps.filter((s) => !s.terminal).map((from) => (
                    <tr key={from.key}>
                      <th className="px-2 py-1 text-left text-xs font-medium">{from.th || from.key}</th>
                      {draft.steps.map((to) => (
                        <td key={to.key} className="px-2 py-1 text-center">
                          {from.key !== to.key && (
                            <input type="checkbox" aria-label={`${from.key} → ${to.key}`} checked={draft.moves.includes(move(from.key, to.key))}
                              onChange={(e) => set({ moves: e.target.checked ? [...draft.moves, move(from.key, to.key)] : draft.moves.filter((m) => m !== move(from.key, to.key)) })} />
                          )}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>

          <section className="space-y-2">
            <h2 className="font-semibold">{t("editor.sla")}</h2>
            <label className="flex items-center gap-2"><input type="checkbox" checked={draft.slaOn} onChange={(e) => set({ slaOn: e.target.checked })} />{t("editor.slaEnabled")}</label>
            {draft.slaOn && (
              <div className="grid gap-3 sm:grid-cols-3">
                <Field label={t("editor.slaCode")}><input className={`${INPUT} font-mono`} value={draft.slaCode} onChange={(e) => set({ slaCode: e.target.value })} /></Field>
                <Field label={t("editor.amount")}><input type="number" min={1} className={`${INPUT}`} value={draft.amount} onChange={(e) => set({ amount: Number(e.target.value) })} /></Field>
                <Field label={t("editor.mode")}>
                  <select className={`${INPUT} bg-white`} value={draft.mode} onChange={(e) => set({ mode: e.target.value as Mode })}>
                    {MODES.map((m) => <option key={m} value={m}>{t(`editor.modes.${m}`)}</option>)}
                  </select>
                </Field>
                <Field label={t("editor.remindBefore")}><input className={`${INPUT}`} value={draft.remind} onChange={(e) => set({ remind: e.target.value })} /></Field>
                <Field label={t("editor.calendar")}>
                  <select className={`${INPUT} bg-white`} value={draft.calendarId} onChange={(e) => set({ calendarId: e.target.value })}>
                    <option value="">{t("editor.defaultCalendar")}</option>
                    {canCalendars && calendars.data?.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
                  </select>
                </Field>
                <div>
                  <span className="block text-slate-600">{t("editor.escalateTo")}</span>
                  <ul className="mt-1 flex flex-wrap gap-1">
                    {draft.escalate.map((r) => (
                      <li key={r.id} className="rounded bg-slate-100 px-2 py-0.5 text-xs">
                        {r.name} <button aria-label={t("editor.remove")} onClick={() => set({ escalate: draft.escalate.filter((x) => x.id !== r.id) })}>×</button>
                      </li>
                    ))}
                  </ul>
                  <Picker kind="user" label="" onPick={(r) => !draft.escalate.some((x) => x.id === r.id) && set({ escalate: [...draft.escalate, r] })} />
                </div>
              </div>
            )}
          </section>

          {save.isError && <p className="text-red-700" role="alert">{t("editor.saveError", { detail: problemText(save.error) })}</p>}
          {savedVersion && <p className="text-emerald-800" role="status">{t("editor.saved", { version: savedVersion })}</p>}
          {editable && (
            <div className="flex gap-2">
              <Button onClick={submit} disabled={save.isPending}>{draft.id ? t("editor.save") : t("editor.saveNew")}</Button>
              <Button variant="secondary" onClick={() => { save.reset(); setDraft(null); }}>{t("editor.cancel")}</Button>
            </div>
          )}
        </fieldset>
      )}
    </main>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="block"><span className="block text-slate-600">{label}</span>{children}</label>;
}

/** Type-ahead picker for a user (active users of the tenant) or a group. */
function Picker({ kind, value, label, onPick }: { kind: "user" | "group"; value?: Ref; label?: string; onPick: (r: Ref) => void }) {
  const t = useTranslations("workflow.editor");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [q, setQ] = useState("");
  const users = useMentionSearch(client, kind === "user" && q.trim() ? q.trim() : null);
  const groups = useGroupSearch(client, kind === "group" && q.trim() ? q.trim() : null);
  const options: Ref[] = kind === "user" ? (users.data ?? []).map((u) => ({ id: u.id, name: u.display_name })) : (groups.data ?? []).map((g) => ({ id: g.id, name: g.name }));
  return (
    <div className="relative">
      {label !== "" && <span className="block text-slate-600">{label ?? (kind === "user" ? t("user") : t("group"))}{value && <span className="ml-2 font-medium text-slate-900" data-testid="picked">{value.name}</span>}</span>}
      <input className={`${INPUT}`} value={q} placeholder={kind === "user" ? t("searchUser") : t("searchGroup")} onChange={(e) => setQ(e.target.value)} />
      {q.trim() && options.length > 0 && (
        <ul role="listbox" className="absolute z-10 mt-1 w-full rounded-md border border-slate-200 bg-white shadow">
          {options.map((o) => (
            <li key={o.id} role="option" aria-selected={false}>
              <button className="w-full px-2 py-1 text-left hover:bg-slate-50" onClick={() => { onPick(o); setQ(""); }}>{o.name}</button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
