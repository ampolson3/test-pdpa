"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useExternalPartyDuplicates,
  useExternalPartyMutations,
  useExternalParties,
  type ExternalParty,
  type ExternalPartyType,
} from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const TYPES: ExternalPartyType[] = ["processor", "recipient", "controller", "joint_controller", "government", "other"];

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

type Draft = {
  id?: string; rowVersion?: number; party_type: ExternalPartyType; name_th: string; name_en: string; registration_no: string;
  country_code: string; contact_name: string; contact_email: string; contact_phone: string; website: string; status: "active" | "inactive";
};

const blank: Draft = { party_type: "processor", name_th: "", name_en: "", registration_no: "", country_code: "TH", contact_name: "",
  contact_email: "", contact_phone: "", website: "", status: "active" };

function fromParty(p: ExternalParty): Draft {
  return { id: p.id, rowVersion: p.row_version, party_type: p.party_type, name_th: p.name_th, name_en: p.name_en ?? "",
    registration_no: p.registration_no ?? "", country_code: p.country_code, contact_name: p.contact?.name ?? "",
    contact_email: p.contact?.email ?? "", contact_phone: p.contact?.phone ?? "", website: p.website ?? "", status: p.status };
}

export function ExternalPartiesContent() {
  const t = useTranslations("externalParties");
  const canRead = usePermission("org.party.read");
  const canCreate = usePermission("org.party.create");
  const canUpdate = usePermission("org.party.update");
  const canMerge = usePermission("org.party.delete");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [typeFilter, setTypeFilter] = useState<ExternalPartyType | "">("");
  const [q, setQ] = useState("");
  const list = useExternalParties(client, { party_type: typeFilter || undefined, q: q || undefined });
  const m = useExternalPartyMutations(client);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [saved, setSaved] = useState(false);
  const [showDuplicates, setShowDuplicates] = useState(false);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const set = (p: Partial<Draft>) => { setSaved(false); setDraft({ ...(draft ?? blank), ...p }); };
  const editable = draft && (draft.id ? canUpdate : canCreate);

  const submit = () => {
    if (!draft) return;
    m.save.mutate({
      id: draft.id, rowVersion: draft.rowVersion,
      input: {
        party_type: draft.party_type, name_th: draft.name_th, name_en: draft.name_en || undefined, registration_no: draft.registration_no || undefined,
        country_code: draft.country_code, website: draft.website || undefined, status: draft.status,
        contact: (draft.contact_name || draft.contact_email || draft.contact_phone)
          ? { name: draft.contact_name || undefined, email: draft.contact_email || undefined, phone: draft.contact_phone || undefined }
          : undefined,
      },
    }, { onSuccess: () => { setDraft(null); setSaved(true); } });
  };
  const field = (key: keyof Draft, label: string, extra?: string) => (
    <label className={extra}><span className="block text-slate-600">{label}</span>
      <input className={INPUT} value={String(draft?.[key] ?? "")} onChange={(e) => set({ [key]: e.target.value } as Partial<Draft>)} /></label>
  );

  return (
    <main className="mx-auto max-w-6xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        <div className="flex gap-2">
          {canMerge && <Button variant="secondary" onClick={() => setShowDuplicates(!showDuplicates)}>{showDuplicates ? t("hideDuplicates") : t("findDuplicates")}</Button>}
          {canCreate && <Button onClick={() => { m.save.reset(); setSaved(false); setDraft({ ...blank }); }}>{t("newParty")}</Button>}
        </div>
      </header>

      {showDuplicates && canMerge && <Duplicates client={client} onMerged={() => list.refetch()} />}

      <div className="flex flex-wrap items-end gap-3">
        <label><span className="block text-slate-600">{t("filterType")}</span>
          <select className={INPUT} value={typeFilter} onChange={(e) => setTypeFilter(e.target.value as ExternalPartyType | "")}>
            <option value="">{t("allTypes")}</option>
            {TYPES.map((x) => <option key={x} value={x}>{t(`types.${x}`)}</option>)}
          </select>
        </label>
        <label className="flex-1"><span className="block text-slate-600">{t("search")}</span>
          <input className={INPUT} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("searchHint")} /></label>
      </div>

      {draft && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2" disabled={!editable}>
          <legend className="px-1 font-semibold">{t("form.details")}</legend>
          <label><span className="block text-slate-600">{t("form.partyType")}</span>
            <select className={INPUT} value={draft.party_type} onChange={(e) => set({ party_type: e.target.value as ExternalPartyType })}>
              {TYPES.map((x) => <option key={x} value={x}>{t(`types.${x}`)}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.countryCode")}</span>
            <input className={INPUT} maxLength={2} value={draft.country_code} onChange={(e) => set({ country_code: e.target.value.toUpperCase() })} /></label>
          {field("name_th", t("form.nameTh"))}
          {field("name_en", t("form.nameEn"))}
          {field("registration_no", t("form.registrationNo"))}
          {field("website", t("form.website"))}
          {field("contact_name", t("form.contactName"))}
          {field("contact_email", t("form.contactEmail"))}
          {field("contact_phone", t("form.contactPhone"))}
          <label className="flex items-center gap-1"><input type="checkbox" checked={draft.status === "active"} onChange={(e) => set({ status: e.target.checked ? "active" : "inactive" })} />{t("form.active")}</label>
          {m.save.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(m.save.error) })}</p>}
          {saved && <p className="text-emerald-800 sm:col-span-2" role="status">{t("form.saved")}</p>}
          {editable && (
            <div className="flex gap-2 sm:col-span-2">
              <Button onClick={submit} disabled={m.save.isPending || !draft.name_th.trim() || draft.country_code.length !== 2}>{t("form.save")}</Button>
              <Button variant="secondary" onClick={() => { m.save.reset(); setDraft(null); }}>{t("form.cancel")}</Button>
            </div>
          )}
        </fieldset>
      )}

      {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : rows.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white" data-testid="party-list">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr><th className="px-3 py-2">{t("form.nameTh")}</th><th className="px-3 py-2">{t("form.partyType")}</th><th className="px-3 py-2">{t("form.countryCode")}</th><th className="px-3 py-2" /></tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((p) => (
              <tr key={p.id}>
                <td className="px-3 py-2">{p.name_th}{p.status === "inactive" && <span className="ml-2 text-xs text-slate-500">({t("inactive")}{p.merged_into_id ? ` · ${t("merged")}` : ""})</span>}</td>
                <td className="px-3 py-2">{t(`types.${p.party_type}`)}</td>
                <td className="px-3 py-2">{p.country_code}</td>
                <td className="px-3 py-2 text-right">
                  {canUpdate && !p.merged_into_id && <button className="text-sky-700 underline" onClick={() => { m.save.reset(); setSaved(false); setDraft(fromParty(p)); }}>{t("edit")}</button>}
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

function Duplicates({ client, onMerged }: { client: ReturnType<typeof createApiClient>; onMerged: () => void }) {
  const t = useTranslations("externalParties");
  const dups = useExternalPartyDuplicates(client);
  const m = useExternalPartyMutations(client);
  const groups = dups.data ?? [];

  if (dups.isPending) return <p className="text-slate-500">{t("loading")}</p>;
  if (groups.length === 0) return <p className="rounded-md bg-emerald-50 p-3 text-emerald-800">{t("noDuplicates")}</p>;

  return (
    <div className="space-y-3 rounded-md border border-amber-200 bg-amber-50 p-4" data-testid="duplicates">
      <h2 className="font-semibold text-amber-900">{t("duplicatesTitle")}</h2>
      {groups.map((g) => (
        <div key={g.dedupe_key} className="rounded-md border border-amber-300 bg-white p-3">
          <ul className="divide-y divide-slate-100">
            {g.parties.map((p) => (
              <li key={p.id} className="flex items-center justify-between gap-2 py-1">
                <span>{p.name_th} <span className="text-xs text-slate-500">({t(`types.${p.party_type}`)}, {p.country_code})</span></span>
                <select
                  className="rounded border border-slate-300 bg-white px-2 py-1 text-xs"
                  value=""
                  onChange={(e) => {
                    if (!e.target.value) return;
                    m.merge.mutate({ sourceId: p.id, targetId: e.target.value }, { onSuccess: onMerged });
                  }}
                >
                  <option value="">{t("mergeInto")}</option>
                  {g.parties.filter((x) => x.id !== p.id).map((x) => <option key={x.id} value={x.id}>{x.name_th}</option>)}
                </select>
              </li>
            ))}
          </ul>
        </div>
      ))}
      {m.merge.isError && <p className="text-red-700" role="alert">{t("mergeError")}</p>}
    </div>
  );
}
