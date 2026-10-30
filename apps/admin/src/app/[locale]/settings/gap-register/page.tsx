import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { GapRegisterContent } from "./gap-register-content";

/** RRA-04: tenant-wide register of every open legal-gap finding (admin: /settings/gap-register). */
export default async function GapRegisterPage() {
  const t = await getTranslations("gapRegister");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <GapRegisterContent />
    </GrantsProvider>
  );
}
