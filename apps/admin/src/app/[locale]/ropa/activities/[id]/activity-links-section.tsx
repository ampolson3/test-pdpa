"use client";

import { useTranslations } from "next-intl";
import { type ApiClient, type ProcessingActivity, useActivityLinks } from "@pdpa/api-client";

/** ROPA-19 — "แท็บความเชื่อมโยงในหน้ากิจกรรม": every other module's record already linked back to this
 *  activity. Data Flow isn't listed here — its diagram (DFG-01) is always computed live off the current
 *  RoPA/org/vendor rows, so there is nothing stale to show; the activity simply always shows up there. */
export function ActivityLinksSection({ client, activity }: { client: ApiClient; activity: ProcessingActivity }) {
  const t = useTranslations("activities");
  const links = useActivityLinks(client, activity.id);

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="links-section">
      <h2 className="font-semibold">{t("links.title")}</h2>
      <p className="text-sm text-slate-500">{t("links.hint")}</p>

      <div>
        <h3 className="font-medium">{t("links.notices")}</h3>
        {(links.data?.notices.length ?? 0) === 0 ? (
          <p className="text-slate-500">{t("links.noNotices")}</p>
        ) : (
          <ul className="divide-y divide-slate-100" data-testid="linked-notices">
            {links.data!.notices.map((n) => (
              <li key={n.id} className="flex items-center justify-between py-2">
                <span>{n.title}</span>
                <span className="rounded bg-slate-100 px-2 py-0.5 text-xs">{n.status}</span>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div>
        <h3 className="font-medium">{t("links.consentPurposes")}</h3>
        {(links.data?.consent_purposes.length ?? 0) === 0 ? (
          <p className="text-slate-500">{t("links.noConsentPurposes")}</p>
        ) : (
          <ul className="divide-y divide-slate-100" data-testid="linked-consent-purposes">
            {links.data!.consent_purposes.map((c) => (
              <li key={c.purpose_id} className="flex items-center justify-between py-2">
                <span>{c.code}</span>
                <span className="rounded bg-slate-100 px-2 py-0.5 text-xs">{c.status}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
