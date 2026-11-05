"use client";

import { useActionState } from "react";
import { useTranslations } from "next-intl";
import type { CampaignView } from "@/lib/campaigns";
import { respondAction, type RespondState } from "./actions";

export function CampaignForm({ view, publicKey, locale, token, idem }: { view: CampaignView; publicKey: string; locale: string; token: string; idem: string }) {
  const t = useTranslations("portal.campaign");
  const [state, action, pending] = useActionState(respondAction.bind(null, publicKey, locale, token, view), { idem } as RespondState);

  if (view.responded || state.receipt) {
    return (
      <section className="space-y-2 rounded-lg bg-white p-6 shadow-sm" role="status" data-testid="campaign-done">
        <h1 className="text-lg font-semibold">{t("thanks")}</h1>
        {state.receipt && <p>{t("receipt", { receipt: state.receipt })}</p>}
      </section>
    );
  }

  return (
    <form action={action} className="space-y-5 rounded-lg bg-white p-6 shadow-sm">
      <header>
        <h1 className="text-lg font-semibold">{view.campaign_name}</h1>
        <p className="text-sm text-slate-600">{t("intro")}</p>
      </header>
      <ul className="space-y-4">
        {view.purposes.map((p) => (
          <li key={p.code} className={`space-y-2 rounded-md border p-4 ${p.is_sensitive ? "border-rose-200" : "border-slate-200"}`} data-testid={`campaign-purpose-${p.code}`}>
            <h2 className="font-medium">{p.name}</h2>
            <p className="text-sm whitespace-pre-line">{p.is_sensitive && p.explicit_text ? p.explicit_text : p.text}</p>
            <div className="flex gap-4">
              <label className="flex items-center gap-1">
                <input type="radio" name={`decision:${p.code}`} value="yes" data-testid={`campaign-yes-${p.code}`} />
                {t("agree")}
              </label>
              <label className="flex items-center gap-1">
                <input type="radio" name={`decision:${p.code}`} value="no" data-testid={`campaign-no-${p.code}`} />
                {t("decline")}
              </label>
            </div>
          </li>
        ))}
      </ul>
      {state.error && (
        <p className="text-sm text-red-700" role="alert" data-testid="campaign-error">
          {t.has(`errors.${state.error}`) ? t(`errors.${state.error}`) : t("errors.generic")}
        </p>
      )}
      <button type="submit" disabled={pending} className="rounded-md bg-slate-900 px-4 py-2 text-white disabled:opacity-50" data-testid="campaign-submit">
        {pending ? t("sending") : t("submit")}
      </button>
    </form>
  );
}
