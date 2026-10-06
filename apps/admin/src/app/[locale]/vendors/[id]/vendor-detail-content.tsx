"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { createApiClient, useVendor, useSaveVendor, useExternalParties } from "@pdpa/api-client";
import { Link } from "@/i18n/routing";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

const STATUS_STYLE: Record<string, string> = {
  prospect: "bg-slate-100 text-slate-700",
  onboarding: "bg-sky-100 text-sky-800",
  approved: "bg-emerald-100 text-emerald-800",
  conditional: "bg-amber-100 text-amber-800",
  rejected: "bg-red-100 text-red-800",
  offboarding: "bg-amber-100 text-amber-800",
  terminated: "bg-slate-100 text-slate-500",
};

/** VEN-01's acceptance criterion: one page per vendor gathering its profile — sibling features (VEN-02
 *  tiering, VEN-04/05/07 assessments, VEN-08 approval, VEN-11 contracts/activities, ...) add their own
 *  sections to this same page once built, rather than scattering vendor data across separate screens. */
export function VendorDetailContent({ id }: { id: string }) {
  const t = useTranslations("vendors");
  const canRead = usePermission("vendor.vendor.read");
  const canUpdate = usePermission("vendor.vendor.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const vendor = useVendor(client, id);
  const parties = useExternalParties(client);
  const save = useSaveVendor(client);
  const [editing, setEditing] = useState(false);
  const [serviceDescription, setServiceDescription] = useState("");
  const [isProcessor, setIsProcessor] = useState(true);
  const [countries, setCountries] = useState("");

  if (!canRead) return <main className="mx-auto max-w-4xl p-8 text-slate-600">{t("forbidden")}</main>;
  if (vendor.isPending) return <main className="mx-auto max-w-4xl p-8 text-slate-500">{t("loading")}</main>;
  if (vendor.isError || !vendor.data) return <main className="mx-auto max-w-4xl p-8 text-red-700">{t("loadError")}</main>;

  const v = vendor.data;
  const partyRows = parties.data?.pages.flatMap((p) => p.data) ?? [];
  const party = partyRows.find((p) => p.id === v.party_id);

  const startEdit = () => {
    save.reset();
    setServiceDescription(v.service_description);
    setIsProcessor(v.is_processor);
    setCountries(v.processing_countries.join(", "));
    setEditing(true);
  };

  const submit = () => {
    save.mutate(
      {
        id: v.id, rowVersion: v.row_version,
        input: {
          party_id: v.party_id, service_description: serviceDescription, is_processor: isProcessor,
          relationship_owner_id: v.relationship_owner_id ?? undefined,
          processing_countries: countries.split(",").map((c) => c.trim()).filter(Boolean),
        },
      },
      { onSuccess: () => setEditing(false) },
    );
  };

  return (
    <main className="mx-auto max-w-4xl space-y-6 p-8 text-sm">
      <Link className="text-sky-700 underline" href="/vendors">{t("backToList")}</Link>

      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{party?.name_th ?? v.party_id}</h1>
          <p className="text-slate-600">{v.service_description}</p>
        </div>
        <span className={`rounded px-2 py-1 ${STATUS_STYLE[v.status]}`} data-testid="vendor-status">{t(`statuses.${v.status}`)}</span>
      </header>

      <section className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2">
        <h2 className="font-semibold sm:col-span-2">{t("form.details")}</h2>
        <div><span className="block text-slate-500">{t("form.party")}</span><span>{party?.name_th ?? "—"}</span></div>
        <div><span className="block text-slate-500">{t("form.tier")}</span><span>{v.tier ? t(`tiers.${v.tier}`) : t("form.tierNotSet")}</span></div>
        <div><span className="block text-slate-500">{t("form.isProcessor")}</span><span>{v.is_processor ? t("statuses.approved") : "—"}</span></div>
        <div><span className="block text-slate-500">{t("form.processingCountries")}</span><span>{v.processing_countries.join(", ") || "—"}</span></div>
        {v.next_assessment_at && <div><span className="block text-slate-500">{t("form.nextAssessmentAt")}</span><span>{v.next_assessment_at}</span></div>}

        {editing ? (
          <>
            <label className="sm:col-span-2"><span className="block text-slate-600">{t("form.serviceDescription")}</span>
              <textarea className={INPUT} rows={2} value={serviceDescription} maxLength={2000}
                onChange={(e) => setServiceDescription(e.target.value)} /></label>
            <label><span className="block text-slate-600">{t("form.processingCountries")}</span>
              <input className={INPUT} value={countries} onChange={(e) => setCountries(e.target.value)} /></label>
            <label className="flex items-center gap-2">
              <input type="checkbox" checked={isProcessor} onChange={(e) => setIsProcessor(e.target.checked)} />
              <span className="text-slate-600">{t("form.isProcessor")}</span>
            </label>
            {save.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(save.error) })}</p>}
            <div className="flex gap-2 sm:col-span-2">
              <Button onClick={submit} disabled={save.isPending}>{t("form.save")}</Button>
              <Button variant="secondary" onClick={() => setEditing(false)}>{t("form.cancel")}</Button>
            </div>
          </>
        ) : (
          canUpdate && <div className="sm:col-span-2"><Button variant="secondary" onClick={startEdit}>{t("form.edit")}</Button></div>
        )}
      </section>
    </main>
  );
}
