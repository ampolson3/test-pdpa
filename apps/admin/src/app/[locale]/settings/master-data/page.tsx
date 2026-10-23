import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { MasterDataContent } from "./master-data-content";

/** ORG-07 master data (admin: /settings/master-data). Gated on org.masterdata.read in MasterDataContent. */
export default async function MasterDataPage() {
  const t = await getTranslations("masterData");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <MasterDataContent />
    </GrantsProvider>
  );
}
