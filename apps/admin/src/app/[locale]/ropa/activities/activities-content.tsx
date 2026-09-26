"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useActivities,
  useActivityMutations,
  useLegalEntities,
  useOrgUnits,
  type ActivityRole,
  type ActivityStatus,
} from "@pdpa/api-client";
import { Link, useRouter } from "@/i18n/routing";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const STATUSES: ActivityStatus[] = ["draft", "pending_approval", "active", "under_review", "ended"];
const ROLES: ActivityRole[] = ["controller", "processor"];

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

type Draft = { legal_entity_id: string; org_unit_id: string; code: string; name: string; role: ActivityRole | "" };
const blank: Draft = { legal_entity_id: "", org_unit_id: "", code: "", name: "", role: "" };

export function ActivitiesContent() {
  const t = useTranslations("activities");
  const canRead = usePermission("ropa.activity.read");
  const canCreate = usePermission("ropa.activity.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const router = useRouter();
  const [statusFilter, setStatusFilter] = useState<ActivityStatus | "">("");
  const [orgUnitFilter, setOrgUnitFilter] = useState("");
  const [q, setQ] = useState("");
  const [legalEntityId, setLegalEntityId] = useState<string>();
  const [draft, setDraft] = useState<Draft | null>(null);

  const list = useActivities(client, { status: statusFilter || undefined, org_unit_id: orgUnitFilter || undefined, q: q || undefined });
  const { save } = useActivityMutations(client);
  const entities = useLegalEntities(client);
  const units = useOrgUnits(client, legalEntityId);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const set = (p: Partial<Draft>) => setDraft({ ...(draft ?? blank), ...p });

  const submit = () => {
    if (!draft || !draft.role) return;
    save.mutate(
      { input: { legal_entity_id: draft.legal_entity_id, org_unit_id: draft.org_unit_id, code: draft.code, name: draft.name, role: draft.role } },
      { onSuccess: (a) => { setDraft(null); router.push(`/ropa/activities/${a!.id}`); } },
    );
  };

  return (
    <main className="mx-auto max-w-6xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        {canCreate && <Button onClick={() => { save.reset(); setDraft({ ...blank }); }}>{t("newActivity")}</Button>}
      </header>

      <div className="flex flex-wrap items-end gap-3">
        <label><span className="block text-slate-600">{t("filterStatus")}</span>
          <select className={INPUT} value={statusFilter} onChange={(e) => setStatusFilter(e.target.value as ActivityStatus | "")}>
            <option value="">{t("allStatuses")}</option>
            {STATUSES.map((s) => <option key={s} value={s}>{t(`statuses.${s}`)}</option>)}
          </select>
        </label>
        <label><span className="block text-slate-600">{t("form.legalEntity")}</span>
          <select className={INPUT} value={legalEntityId ?? ""} onChange={(e) => { setLegalEntityId(e.target.value || undefined); setOrgUnitFilter(""); }}>
            <option value="">{t("form.none")}</option>
            {entities.data?.map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
          </select>
        </label>
        <label><span className="block text-slate-600">{t("filterUnit")}</span>
          <select className={INPUT} value={orgUnitFilter} onChange={(e) => setOrgUnitFilter(e.target.value)} disabled={!legalEntityId}>
            <option value="">{t("allUnits")}</option>
            {units.data?.map((u) => <option key={u.id} value={u.id}>{u.name_th}</option>)}
          </select>
        </label>
        <label><span className="block text-slate-600">{t("filterQuery")}</span>
          <input className={INPUT} value={q} onChange={(e) => setQ(e.target.value)} /></label>
      </div>

      {draft && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2" disabled={!canCreate}>
          <legend className="px-1 font-semibold">{t("form.details")}</legend>
          <label><span className="block text-slate-600">{t("form.legalEntity")}</span>
            <select className={INPUT} value={draft.legal_entity_id} onChange={(e) => set({ legal_entity_id: e.target.value })}>
              <option value="">{t("form.choose")}</option>
              {entities.data?.map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.orgUnit")}</span>
            <select className={INPUT} value={draft.org_unit_id} onChange={(e) => set({ org_unit_id: e.target.value })}>
              <option value="">{t("form.choose")}</option>
              {units.data?.map((u) => <option key={u.id} value={u.id}>{u.name_th}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.code")}</span>
            <input className={INPUT} value={draft.code} onChange={(e) => set({ code: e.target.value })} maxLength={40} /></label>
          <label><span className="block text-slate-600">{t("form.name")}</span>
            <input className={INPUT} value={draft.name} onChange={(e) => set({ name: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.role")}</span>
            <select className={INPUT} value={draft.role} onChange={(e) => set({ role: e.target.value as ActivityRole })}>
              <option value="">{t("form.choose")}</option>
              {ROLES.map((r) => <option key={r} value={r}>{t(`roles.${r}`)}</option>)}
            </select>
          </label>
          {save.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(save.error) })}</p>}
          <div className="flex gap-2 sm:col-span-2">
            <Button onClick={submit} disabled={save.isPending || !draft.legal_entity_id || !draft.org_unit_id || !draft.code || !draft.name || !draft.role}>{t("form.save")}</Button>
            <Button variant="secondary" onClick={() => { save.reset(); setDraft(null); }}>{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : rows.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white" data-testid="activities-list">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr><th className="px-3 py-2">{t("form.code")}</th><th className="px-3 py-2">{t("form.name")}</th><th className="px-3 py-2">{t("form.role")}</th>
              <th className="px-3 py-2">{t("statusLabel")}</th><th className="px-3 py-2">{t("completeness")}</th></tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((a) => (
              <tr key={a.id}>
                <td className="px-3 py-2 font-mono text-xs"><Link className="text-sky-700 underline" href={`/ropa/activities/${a.id}`}>{a.code}</Link></td>
                <td className="px-3 py-2">{a.name}</td>
                <td className="px-3 py-2">{t(`roles.${a.role}`)}</td>
                <td className="px-3 py-2">{t(`statuses.${a.status}`)}</td>
                <td className="px-3 py-2">
                  <span className={a.completeness < 100 ? "rounded bg-amber-100 px-2 py-0.5 text-amber-800" : "rounded bg-emerald-100 px-2 py-0.5 text-emerald-800"} data-testid="completeness-badge">
                    {a.completeness < 100 ? t("incomplete", { pct: a.completeness }) : t("complete")}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {list.hasNextPage && <Button variant="secondary" onClick={() => list.fetchNextPage()}>{t("more")}</Button>}
    </main>
  );
}
