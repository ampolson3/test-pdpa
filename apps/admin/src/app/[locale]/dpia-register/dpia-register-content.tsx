"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { createApiClient, useDpiaRegistry, useLegalEntities, useOrgUnits, type DpiaRegistryEntry } from "@pdpa/api-client";
import { Link } from "@/i18n/routing";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
// "screening" is excluded: DPIA-01/02's own transient status is never actually persisted on a row
// (docs/modules/DPIA.md's own note), so no real registry entry can ever have it.
const STATUSES = ["not_required", "in_progress", "in_review", "approved", "rejected", "needs_review", "closed"] as const;

function statusBadge(status: DpiaRegistryEntry["status"]): string {
  switch (status) {
    case "not_required":
    case "closed":
    case "approved":
      return "bg-emerald-100 text-emerald-800";
    case "rejected":
      return "bg-red-100 text-red-800";
    case "needs_review":
    case "in_review":
      return "bg-amber-100 text-amber-800";
    default:
      return "bg-sky-100 text-sky-800";
  }
}

/** DPIA-12: every RoPA activity's current DPIA round in one report, read live so the status shown always
 *  matches each DPIA's real current state (no cached copy — Registry() on the backend re-reads every call). */
export function DpiaRegisterContent() {
  const t = useTranslations("dpia");
  const canRead = usePermission("assessment.dpia.read");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [legalEntityId, setLegalEntityId] = useState<string>();
  const [orgUnitId, setOrgUnitId] = useState<string>();
  const [status, setStatus] = useState<DpiaRegistryEntry["status"]>();

  const entities = useLegalEntities(client);
  const units = useOrgUnits(client, legalEntityId);
  const registry = useDpiaRegistry(client, { legal_entity_id: legalEntityId, org_unit_id: orgUnitId, status });

  if (!canRead) return <main className="mx-auto max-w-6xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = registry.data?.data ?? [];

  return (
    <main className="mx-auto max-w-6xl space-y-6 p-8 text-sm">
      <header>
        <h1 className="text-xl font-semibold">{t("registry.title")}</h1>
        <p className="text-slate-600">{t("registry.intro")}</p>
      </header>

      <div className="flex flex-wrap items-end gap-3">
        <label><span className="block text-slate-600">{t("registry.legalEntity")}</span>
          <select className={INPUT} value={legalEntityId ?? ""} onChange={(e) => { setLegalEntityId(e.target.value || undefined); setOrgUnitId(undefined); }}>
            <option value="">{t("registry.allEntities")}</option>
            {entities.data?.map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
          </select>
        </label>
        <label><span className="block text-slate-600">{t("registry.orgUnit")}</span>
          <select className={INPUT} value={orgUnitId ?? ""} onChange={(e) => setOrgUnitId(e.target.value || undefined)} disabled={!legalEntityId}>
            <option value="">{t("registry.allUnits")}</option>
            {units.data?.map((u) => <option key={u.id} value={u.id}>{u.name_th}</option>)}
          </select>
        </label>
        <label><span className="block text-slate-600">{t("registry.status")}</span>
          <select className={INPUT} value={status ?? ""} onChange={(e) => setStatus((e.target.value || undefined) as DpiaRegistryEntry["status"] | undefined)}>
            <option value="">{t("registry.allStatuses")}</option>
            {STATUSES.map((s) => <option key={s} value={s}>{t(`decision.status.${s}`)}</option>)}
          </select>
        </label>
      </div>

      {registry.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : registry.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : rows.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("registry.empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white" data-testid="dpia-registry-list">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr>
              <th className="px-3 py-2">{t("registry.activity")}</th>
              <th className="px-3 py-2">{t("registry.legalEntityCol")}</th>
              <th className="px-3 py-2">{t("registry.orgUnitCol")}</th>
              <th className="px-3 py-2">{t("registry.round")}</th>
              <th className="px-3 py-2">{t("registry.statusCol")}</th>
              <th className="px-3 py-2">{t("registry.result")}</th>
              <th className="px-3 py-2" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((r) => (
              <tr key={r.assessment_id} data-testid="dpia-registry-row">
                <td className="px-3 py-2">{r.activity_name} <span className="text-slate-400">({r.activity_code})</span></td>
                <td className="px-3 py-2">{r.legal_entity_name}</td>
                <td className="px-3 py-2">{r.org_unit_name}</td>
                <td className="px-3 py-2">{r.round_no}</td>
                <td className="px-3 py-2">
                  <span className={`rounded px-2 py-0.5 text-xs ${statusBadge(r.status)}`} data-testid="dpia-registry-status">{t(`decision.status.${r.status}`)}</span>
                </td>
                <td className="px-3 py-2">{t(`results.${r.screening_result}`)}</td>
                <td className="px-3 py-2 text-right">
                  <Link className="text-sky-700 underline" href={`/ropa/activities/${r.activity_id}`}>{t("registry.open")}</Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
