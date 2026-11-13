import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { CampaignsContent } from "./campaigns-content";

/** CON-24 bulk consent request campaigns (admin: /consent/campaigns). Gated on consent.campaign.read in CampaignsContent. */
export default async function CampaignsPage() {
  const t = await getTranslations("consent");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <CampaignsContent />
    </GrantsProvider>
  );
}
