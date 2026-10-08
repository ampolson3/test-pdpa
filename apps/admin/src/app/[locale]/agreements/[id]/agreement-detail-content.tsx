"use client";

import { useMemo } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { createApiClient, useAgreement } from "@pdpa/api-client";
import { Link } from "@/i18n/routing";

const STATUS_STYLE: Record<string, string> = {
  draft: "bg-slate-100 text-slate-700",
  active: "bg-emerald-100 text-emerald-800",
  expired: "bg-amber-100 text-amber-800",
  terminated: "bg-slate-100 text-slate-500",
};

/** DPA-02's own agreement — editing/exporting/publishing the composed content happens on the linked
 *  PLT-16 document page; this page is the agreement's own metadata (parties, activities, renewal). */
export function AgreementDetailContent({ id }: { id: string }) {
  const t = useTranslations("agreements");
  const canRead = usePermission("agreement.dpa.read");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const agreement = useAgreement(client, id);

  if (!canRead) return <main className="mx-auto max-w-4xl p-8 text-slate-600">{t("forbidden")}</main>;
  if (agreement.isPending) return <main className="mx-auto max-w-4xl p-8 text-slate-500">{t("loading")}</main>;
  if (agreement.isError || !agreement.data) return <main className="mx-auto max-w-4xl p-8 text-red-700">{t("loadError")}</main>;

  const a = agreement.data;

  return (
    <main className="mx-auto max-w-4xl space-y-6 p-8 text-sm">
      <Link className="text-sky-700 underline" href="/agreements">{t("backToList")}</Link>

      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{a.title}</h1>
          <p className="text-slate-600">{a.agreement_no}</p>
        </div>
        <span className={`rounded px-2 py-1 text-xs ${STATUS_STYLE[a.status] ?? "bg-slate-100 text-slate-700"}`}>{t(`statuses.${a.status}`)}</span>
      </header>

      <dl className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2">
        <div><dt className="text-slate-500">{t("table.type")}</dt><dd>{t(`types.${a.agreement_type}`)}</dd></div>
        <div><dt className="text-slate-500">{t("table.ourRole")}</dt><dd>{t(`roles.${a.our_role}`)}</dd></div>
        <div><dt className="text-slate-500">{t("detail.autoRenew")}</dt><dd>{a.auto_renew ? t("detail.yes") : t("detail.no")}</dd></div>
        <div><dt className="text-slate-500">{t("detail.renewalNoticeDays")}</dt><dd>{a.renewal_notice_days}</dd></div>
        {a.effective_from && <div><dt className="text-slate-500">{t("detail.effectiveFrom")}</dt><dd>{a.effective_from}</dd></div>}
      </dl>

      <section className="space-y-2">
        <h2 className="font-semibold">{t("detail.activities")}</h2>
        {a.activity_ids.length === 0 ? (
          <p className="text-slate-500">{t("detail.noActivities")}</p>
        ) : (
          <ul className="list-inside list-disc rounded-md border border-slate-200 bg-white p-3">
            {a.activity_ids.map((id) => (
              <li key={id}><Link className="text-sky-700 underline" href={`/ropa/activities/${id}`}>{id}</Link></li>
            ))}
          </ul>
        )}
      </section>

      <Link className="inline-block rounded-md bg-sky-700 px-3 py-1.5 text-white" href={`/documents/${a.document_id}`}>{t("detail.openDocument")}</Link>
    </main>
  );
}
