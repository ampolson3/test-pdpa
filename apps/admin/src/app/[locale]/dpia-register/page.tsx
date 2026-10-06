import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DpiaRegisterContent } from "./dpia-register-content";

/** DPIA-12 register & status (admin: /dpia-register). Gated on assessment.dpia.read in DpiaRegisterContent. */
export default async function DpiaRegisterPage() {
  const t = await getTranslations("dpia");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-6xl p-8 text-slate-600">{t("forbidden")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DpiaRegisterContent />
    </GrantsProvider>
  );
}
