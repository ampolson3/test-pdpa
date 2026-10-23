"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useVendors,
  useSaveVendor,
  useExternalParties,
  useMentionSearch,
  type VendorStatus,
} from "@pdpa/api-client";
import { Link } from "@/i18n/routing";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const STATUSES: VendorStatus[] = ["prospect", "onboarding", "approved", "conditional", "rejected", "offboarding", "terminated"];

const STATUS_STYLE: Record<VendorStatus, string> = {
  prospect: "bg-slate-100 text-slate-700",
  onboarding: "bg-sky-100 text-sky-800",
  approved: "bg-emerald-100 text-emerald-800",
  conditional: "bg-amber-100 text-amber-800",
  rejected: "bg-red-100 text-red-800",
  offboarding: "bg-amber-100 text-amber-800",
  terminated: "bg-slate-100 text-slate-500",
};

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

function OwnerPicker({ client, onPick }: { client: ReturnType<typeof createApiClient>; onPick: (u: { id: string; display_name: string }) => void }) {
  const t = useTranslations("vendors");
  const [q, setQ] = useState("");
  const matches = useMentionSearch(client, q.length >= 2 ? q : null);
  return (
    <div className="relative inline-block w-full">
      <input className={INPUT} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("form.ownerSearch")} />
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

type Draft = {
  party_id: string; service_description: string; relationship_owner_id: string; relationship_owner_name: string;
  is_processor: boolean; processing_countries: string;
};

const blank: Draft = { party_id: "", service_description: "", relationship_owner_id: "", relationship_owner_name: "", is_processor: true, processing_countries: "" };

export function VendorsContent() {
  const t = useTranslations("vendors");
  const canRead = usePermission("vendor.vendor.read");
  const canCreate = usePermission("vendor.vendor.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [statusFilter, setStatusFilter] = useState<VendorStatus | "">("");
  const list = useVendors(client, { status: statusFilter || undefined });
  const parties = useExternalParties(client);
  const save = useSaveVendor(client);
  const [draft, setDraft] = useState<Draft | null>(null);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const partyRows = parties.data?.pages.flatMap((p) => p.data) ?? [];
  const partyName = (id: string) => partyRows.find((p) => p.id === id)?.name_th ?? id;

  const submit = () => {
    if (!draft || !draft.party_id || !draft.service_description.trim()) return;
    save.mutate(
      {
        input: {
          party_id: draft.party_id, service_description: draft.service_description,
          relationship_owner_id: draft.relationship_owner_id || undefined, is_processor: draft.is_processor,
          processing_countries: draft.processing_countries.split(",").map((c) => c.trim()).filter(Boolean),
        },
      },
      { onSuccess: () => setDraft(null) },
    );
  };

  return (
    <main className="mx-auto max-w-6xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
          <Link className="text-sky-700 underline" href="/settings/dpia-templates?type=vendor">{t("assessmentTemplatesLink")}</Link>
        </div>
        {canCreate && <Button onClick={() => { save.reset(); setDraft({ ...blank }); }} data-testid="new-vendor">{t("newVendor")}</Button>}
      </header>

      {draft && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2">
          <legend className="px-1 font-semibold">{t("form.details")}</legend>
          <label><span className="block text-slate-600">{t("form.party")}</span>
            <select className={INPUT} value={draft.party_id} onChange={(e) => setDraft({ ...draft, party_id: e.target.value })}>
              <option value="">{t("form.none")}</option>
              {partyRows.map((p) => <option key={p.id} value={p.id}>{p.name_th}</option>)}
            </select>
          </label>
          <label className="sm:col-span-2"><span className="block text-slate-600">{t("form.serviceDescription")}</span>
            <textarea className={INPUT} rows={2} value={draft.service_description} maxLength={2000}
              onChange={(e) => setDraft({ ...draft, service_description: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.relationshipOwner")}</span>
            {draft.relationship_owner_name ? (
              <span className="mt-1 flex items-center gap-1 rounded bg-slate-100 px-2 py-1">{draft.relationship_owner_name}
                <button type="button" className="text-slate-500" onClick={() => setDraft({ ...draft, relationship_owner_id: "", relationship_owner_name: "" })}>×</button></span>
            ) : <OwnerPicker client={client} onPick={(u) => setDraft({ ...draft, relationship_owner_id: u.id, relationship_owner_name: u.display_name })} />}
          </label>
          <label><span className="block text-slate-600">{t("form.processingCountries")}</span>
            <input className={INPUT} value={draft.processing_countries} placeholder="TH, SG"
              onChange={(e) => setDraft({ ...draft, processing_countries: e.target.value })} /></label>
          <label className="flex items-center gap-2 sm:col-span-2">
            <input type="checkbox" checked={draft.is_processor} onChange={(e) => setDraft({ ...draft, is_processor: e.target.checked })} />
            <span className="text-slate-600">{t("form.isProcessor")}</span>
          </label>
          {save.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(save.error) })}</p>}
          <div className="flex gap-2 sm:col-span-2">
            <Button onClick={submit} disabled={save.isPending || !draft.party_id || !draft.service_description.trim()}>{t("form.save")}</Button>
            <Button variant="secondary" onClick={() => { save.reset(); setDraft(null); }}>{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <span className="text-slate-600">{t("filterStatus")}</span>
        <select className={INPUT + " w-auto"} value={statusFilter} onChange={(e) => setStatusFilter(e.target.value as VendorStatus | "")}>
          <option value="">{t("allStatuses")}</option>
          {STATUSES.map((s) => <option key={s} value={s}>{t(`statuses.${s}`)}</option>)}
        </select>
      </div>

      {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : rows.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white" data-testid="vendor-list">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr><th className="px-3 py-2">{t("form.party")}</th><th className="px-3 py-2">{t("form.serviceDescription")}</th>
              <th className="px-3 py-2">{t("form.status")}</th><th className="px-3 py-2" /></tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((v) => (
              <tr key={v.id}>
                <td className="px-3 py-2">{partyName(v.party_id)}</td>
                <td className="px-3 py-2">{v.service_description}</td>
                <td className="px-3 py-2"><span className={`rounded px-2 py-0.5 ${STATUS_STYLE[v.status]}`}>{t(`statuses.${v.status}`)}</span></td>
                <td className="px-3 py-2 text-right">
                  <Link className="text-sky-700 underline" href={`/vendors/${v.id}`} data-testid={`open-${v.id}`}>{t("open")}</Link>
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
