import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DpoContent } from "./dpo-content";

/** DPO-01 appointment register (admin: /settings/dpo). Gated on dpo.profile.read in DpoContent. */
export default async function DpoPage() {
  const t = await getTranslations("dpo");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DpoContent />
    </GrantsProvider>
  );
}
