import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { AssetsContent } from "./assets-content";

/** ROPA-02 system/asset register (admin: /ropa/assets). Gated on ropa.inventory.read in AssetsContent. */
export default async function AssetsPage() {
  const t = await getTranslations("assets");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <AssetsContent />
    </GrantsProvider>
  );
}
