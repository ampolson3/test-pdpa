import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { NoticesContent } from "./notices-content";

/** PNG-01 wizard-based privacy notice generator (admin: /notices). Gated on notice.document.read in NoticesContent. */
export default async function NoticesPage() {
  const t = await getTranslations("notices");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <NoticesContent />
    </GrantsProvider>
  );
}
