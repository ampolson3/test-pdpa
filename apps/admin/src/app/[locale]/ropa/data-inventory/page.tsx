import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DataInventoryContent } from "./data-inventory-content";

/** ROPA-01 personal data inventory (admin: /ropa/data-inventory). Gated on ropa.inventory.read in DataInventoryContent. */
export default async function DataInventoryPage() {
  const t = await getTranslations("dataInventory");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DataInventoryContent />
    </GrantsProvider>
  );
}
