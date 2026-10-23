"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { createApiClient, useIncidentMutations, useIncidents, useLegalEntities, type BreachFilter, type BreachIncidentInput } from "@pdpa/api-client";
import { Link, useRouter } from "@/i18n/routing";
import { Clock, RiskBadge, StatusBadge, field, input, problemText, useWhen } from "./shared";

const STATUSES = ["reported", "triage", "assessing", "notifying", "remediating", "closed"] as const;
const TYPES = ["confidentiality", "integrity", "availability"] as const;

export function IncidentsContent() {
  const t = useTranslations("breach");
  const canCreate = usePermission("breach.incident.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [filter, setFilter] = useState<BreachFilter>({ open_only: true });
  const [q, setQ] = useState("");
  const list = useIncidents(client, filter);
  const [creating, setCreating] = useState(false);
  const when = useWhen();

  return (
    <main className="mx-auto max-w-6xl space-y-4 p-8 text-sm">
      <header className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        {canCreate && <Button onClick={() => setCreating(true)} data-testid="new-incident">{t("new")}</Button>}
      </header>
      {creating && <NewIncident onCancel={() => setCreating(false)} />}
      <form className="flex flex-wrap items-end gap-2" onSubmit={(e) => { e.preventDefault(); setFilter({ ...filter, q }); }}>
        <input className={`${field} w-64`} placeholder={t("search")} value={q} onChange={(e) => setQ(e.target.value)} data-testid="incident-search" />
        <select className={`${field} w-44`} value={filter.status ?? ""} onChange={(e) => setFilter({ ...filter, status: (e.target.value || undefined) as BreachFilter["status"] })}>
          <option value="">{t("allStatuses")}</option>
          {STATUSES.map((s) => <option key={s} value={s}>{t(`status.${s}`)}</option>)}
        </select>
        <label className="flex items-center gap-1">
          <input type="checkbox" checked={!!filter.open_only} onChange={(e) => setFilter({ ...filter, open_only: e.target.checked })} />
          {t("openOnly")}
        </label>
        <Button type="submit" variant="secondary">{t("searchButton")}</Button>
      </form>
      {list.isError && <p className="text-red-700">{problemText(list.error)}</p>}
      <div className="overflow-x-auto rounded-md border border-slate-200 bg-white">
        <table className="w-full text-left">
          <thead className="bg-slate-50 text-xs text-slate-500">
            <tr>
              <th className="px-3 py-2">{t("no")}</th><th>{t("incident")}</th><th>{t("statusLabel")}</th><th>{t("riskLabel")}</th>
              <th>{t("awareAt")}</th><th>{t("deadline")}</th><th>{t("owner")}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {list.data?.map((i) => (
              <tr key={i.id} data-testid={`incident-${i.incident_no}`}>
                <td className="px-3 py-2 font-mono text-xs"><Link className="text-sky-700 underline" href={`/incidents/${i.id}`}>{i.incident_no}</Link></td>
                <td className="max-w-xs truncate">{i.title}{i.is_drill && <span className="ml-1 text-xs text-slate-500">({t("drill")})</span>}</td>
                <td><StatusBadge status={i.status} /></td>
                <td><RiskBadge risk={i.risk_level} /></td>
                <td className="whitespace-nowrap text-xs">{when(i.aware_at)}</td>
                <td><Clock incident={i} /></td>
                <td className="text-xs">{i.owner_name}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {list.data?.length === 0 && <p className="p-3 text-slate-500">{t("empty")}</p>}
      </div>
    </main>
  );
}

function localInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function NewIncident({ onCancel }: { onCancel: () => void }) {
  const t = useTranslations("breach");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const entities = useLegalEntities(client);
  const m = useIncidentMutations(client);
  const router = useRouter();
  const [v, setV] = useState({ title: "", description: "", types: ["confidentiality"] as string[], via: "email", aware: localInput(new Date()), subjects: "", entity: "" });
  const entity = v.entity || entities.data?.[0]?.id || "";
  const submit = () => {
    const body: BreachIncidentInput = {
      legal_entity_id: entity, reported_via: v.via as BreachIncidentInput["reported_via"], title: v.title, description: v.description,
      breach_types: v.types as BreachIncidentInput["breach_types"], aware_at: new Date(v.aware).toISOString(),
      ...(v.subjects ? { affected_subjects: Number(v.subjects) } : {}),
    };
    m.create.mutate(body, { onSuccess: (i) => router.push(`/incidents/${i.id}`) });
  };
  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="new-incident-form">
      <h2 className="font-semibold">{t("new")}</h2>
      <p className="rounded-md bg-amber-50 p-2 text-amber-900">{t("awareHint")}</p>
      <div className="grid gap-2 md:grid-cols-2">
        <label className="space-y-0.5 md:col-span-2">
          <span className="font-medium">{t("titleLabel")}</span>
          <input className={input} value={v.title} onChange={(e) => setV({ ...v, title: e.target.value })} data-testid="incident-title" />
        </label>
        <label className="space-y-0.5 md:col-span-2">
          <span className="font-medium">{t("description")}</span>
          <textarea className={input} rows={3} value={v.description} onChange={(e) => setV({ ...v, description: e.target.value })} data-testid="incident-description" />
        </label>
        <label className="space-y-0.5">
          <span className="font-medium">{t("awareAt")}</span>
          <input type="datetime-local" className={input} value={v.aware} onChange={(e) => setV({ ...v, aware: e.target.value })} data-testid="incident-aware" />
        </label>
        <label className="space-y-0.5">
          <span className="font-medium">{t("affectedSubjects")}</span>
          <input type="number" min={0} className={input} value={v.subjects} onChange={(e) => setV({ ...v, subjects: e.target.value })} data-testid="incident-subjects" />
        </label>
        <label className="space-y-0.5">
          <span className="font-medium">{t("reportedVia")}</span>
          <select className={input} value={v.via} onChange={(e) => setV({ ...v, via: e.target.value })}>
            {["employee_form", "email", "phone", "system"].map((x) => <option key={x} value={x}>{t(`via.${x}`)}</option>)}
          </select>
        </label>
        <label className="space-y-0.5">
          <span className="font-medium">{t("legalEntity")}</span>
          <select className={input} value={entity} onChange={(e) => setV({ ...v, entity: e.target.value })}>
            {entities.data?.map((e) => <option key={e.id} value={e.id}>{e.name_th}</option>)}
          </select>
        </label>
      </div>
      <fieldset className="flex flex-wrap gap-3">
        <legend className="font-medium">{t("breachTypes")}</legend>
        {TYPES.map((x) => (
          <label key={x} className="flex items-center gap-1">
            <input type="checkbox" checked={v.types.includes(x)} onChange={(e) => setV({ ...v, types: e.target.checked ? [...v.types, x] : v.types.filter((y) => y !== x) })} />
            {t(`type.${x}`)}
          </label>
        ))}
      </fieldset>
      {m.create.isError && <p className="text-red-700" role="alert">{problemText(m.create.error)}</p>}
      <div className="flex gap-2">
        <Button onClick={submit} disabled={m.create.isPending || !v.title || !v.description || !entity || v.types.length === 0} data-testid="create-incident">{t("create")}</Button>
        <Button variant="ghost" onClick={onCancel}>{t("cancel")}</Button>
      </div>
    </section>
  );
}
