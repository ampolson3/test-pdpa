import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { GovernanceContent } from "./governance-content";

/** DPX-05 retention & disposal management (admin: /governance). Gated on dpx.retention.read in GovernanceContent. */
export default async function GovernancePage() {
  const t = await getTranslations("governance");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <GovernanceContent />
    </GrantsProvider>
  );
}
