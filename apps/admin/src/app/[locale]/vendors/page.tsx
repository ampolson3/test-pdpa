import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { VendorsContent } from "./vendors-content";

/** VEN-01 vendor/processor registry (admin: /vendors). Gated on vendor.vendor.read in VendorsContent. */
export default async function VendorsPage() {
  const t = await getTranslations("vendors");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <VendorsContent />
    </GrantsProvider>
  );
}
