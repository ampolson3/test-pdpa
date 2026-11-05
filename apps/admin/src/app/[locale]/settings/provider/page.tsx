import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { ProviderContent } from "./provider-content";

/** IAM-03 provider console (admin: /settings/provider). Gated on admin.tenant.read in ProviderContent. */
export default async function ProviderPage() {
  const t = await getTranslations("provider");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <ProviderContent />
    </GrantsProvider>
  );
}
