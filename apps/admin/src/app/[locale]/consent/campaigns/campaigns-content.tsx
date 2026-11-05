"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { createApiClient, useConsentCampaignMutations, useConsentCampaigns, useConsentPurposes, type ConsentCampaignInput } from "@pdpa/api-client";
import { input, problemText, useText } from "../shared";

const CHANNELS = ["email", "sms", "line"] as const;
const campaignStatusTone: Record<string, string> = {
  draft: "bg-slate-100 text-slate-700",
  sending: "bg-sky-50 text-sky-800",
  done: "bg-emerald-50 text-emerald-800",
  cancelled: "bg-rose-50 text-rose-800",
};

// CON-24 (ส่งคำขอความยินยอมแบบ Bulk): identifiers are typed one per line as "type:value" (e.g.
// "email:alice@example.com") and resolved server-side into real data subjects at creation — never stored
// raw, so this form never round-trips an identifier back from the API.
function parseIdentifiers(raw: string): ConsentCampaignInput["identifiers"] {
  return raw
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean)
    .map((l) => {
      const i = l.indexOf(":");
      const type = (i < 0 ? "email" : l.slice(0, i).trim()) as ConsentCampaignInput["identifiers"][number]["type"];
      const value = i < 0 ? l : l.slice(i + 1).trim();
      return { type, value };
    })
    .filter((id) => id.value.length > 0);
}

export function CampaignsContent() {
  const t = useTranslations("consent");
  const canRead = usePermission("consent.campaign.read");
  const canCreate = usePermission("consent.campaign.create");
  const canExecute = usePermission("consent.campaign.execute");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const pick = useText();
  const campaigns = useConsentCampaigns(client);
  const purposes = useConsentPurposes(client);
  const { create, send, resend } = useConsentCampaignMutations(client);

  const [showNew, setShowNew] = useState(false);
  const [name, setName] = useState("");
  const [channel, setChannel] = useState<(typeof CHANNELS)[number]>("email");
  const [purposeIds, setPurposeIds] = useState<string[]>([]);
  const [identifiers, setIdentifiers] = useState("");

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const togglePurpose = (id: string) => setPurposeIds((cur) => (cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]));

  const submit = () => {
    const body: ConsentCampaignInput = { name, channel, purpose_ids: purposeIds, identifiers: parseIdentifiers(identifiers) };
    create.mutate(body, {
      onSuccess: () => {
        setShowNew(false);
        setName("");
        setPurposeIds([]);
        setIdentifiers("");
      },
    });
  };

  return (
    <main className="mx-auto max-w-5xl space-y-4 p-8 text-sm">
      <header className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("campaign.title")}</h1>
          <p className="text-slate-600">{t("campaign.intro")}</p>
        </div>
        {canCreate && (
          <Button onClick={() => setShowNew((v) => !v)} data-testid="new-campaign">
            {t("campaign.new")}
          </Button>
        )}
      </header>

      {showNew && (
        <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="campaign-form">
          <label className="block">
            <span className="font-medium">{t("campaign.name")}</span>
            <input className={input} value={name} onChange={(e) => setName(e.target.value)} data-testid="campaign-name" />
          </label>
          <label className="block">
            <span className="font-medium">{t("campaign.channel")}</span>
            <select className={input} value={channel} onChange={(e) => setChannel(e.target.value as (typeof CHANNELS)[number])} data-testid="campaign-channel">
              {CHANNELS.map((c) => (
                <option key={c} value={c}>
                  {t(`campaign.channels.${c}`)}
                </option>
              ))}
            </select>
          </label>
          <fieldset className="space-y-1">
            <legend className="font-medium">{t("campaign.purposes")}</legend>
            <div className="flex flex-wrap gap-3">
              {purposes.data
                ?.filter((p) => p.status === "active")
                .map((p) => (
                  <label key={p.id} className="flex items-center gap-1">
                    <input type="checkbox" checked={purposeIds.includes(p.id)} onChange={() => togglePurpose(p.id)} data-testid={`campaign-purpose-${p.code}`} />
                    {pick(p.live.name) || p.code}
                  </label>
                ))}
            </div>
          </fieldset>
          <label className="block">
            <span className="font-medium">{t("campaign.identifiers")}</span>
            <p className="text-xs text-slate-500">{t("campaign.identifiersHint")}</p>
            <textarea className={input} rows={5} value={identifiers} onChange={(e) => setIdentifiers(e.target.value)} data-testid="campaign-identifiers" />
          </label>
          {create.isError && <p className="text-red-700" data-testid="campaign-error">{problemText(create.error)}</p>}
          <Button onClick={submit} disabled={create.isPending} data-testid="campaign-submit">
            {t("campaign.create")}
          </Button>
        </section>
      )}

      <section className="rounded-md border border-slate-200 bg-white">
        {campaigns.isPending && <p className="p-3 text-slate-500">{t("loading")}</p>}
        {campaigns.data?.length === 0 && <p className="p-3 text-slate-500">{t("campaign.empty")}</p>}
        <table className="w-full text-left">
          <thead className="border-b border-slate-200 text-xs text-slate-500">
            <tr>
              <th className="px-3 py-2">{t("campaign.name")}</th>
              <th className="px-3 py-2">{t("campaign.channel")}</th>
              <th className="px-3 py-2">{t("campaign.status")}</th>
              <th className="px-3 py-2">{t("campaign.sent")}</th>
              <th className="px-3 py-2">{t("campaign.opened")}</th>
              <th className="px-3 py-2">{t("campaign.responded")}</th>
              <th className="px-3 py-2" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {campaigns.data?.map((c) => (
              <tr key={c.id} data-testid={`campaign-row-${c.id}`}>
                <td className="px-3 py-2 font-medium">{c.name}</td>
                <td className="px-3 py-2">{t(`campaign.channels.${c.channel}`)}</td>
                <td className="px-3 py-2">
                  <span className={`rounded-full px-2 py-0.5 text-xs ${campaignStatusTone[c.status] ?? campaignStatusTone.draft}`} data-testid={`campaign-status-${c.id}`}>
                    {t(`campaign.statuses.${c.status}`)}
                  </span>
                </td>
                <td className="px-3 py-2" data-testid={`campaign-sent-${c.id}`}>{c.sent_count}</td>
                <td className="px-3 py-2" data-testid={`campaign-opened-${c.id}`}>{c.opened_count}</td>
                <td className="px-3 py-2" data-testid={`campaign-responded-${c.id}`}>{c.response_count}</td>
                <td className="px-3 py-2 text-right">
                  {canExecute && c.status === "draft" && (
                    <Button onClick={() => send.mutate(c.id)} disabled={send.isPending} data-testid={`campaign-send-${c.id}`}>
                      {t("campaign.send")}
                    </Button>
                  )}
                  {canExecute && c.status !== "draft" && c.status !== "cancelled" && (
                    <Button onClick={() => resend.mutate(c.id)} disabled={resend.isPending} data-testid={`campaign-resend-${c.id}`}>
                      {t("campaign.resend")}
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </main>
  );
}
