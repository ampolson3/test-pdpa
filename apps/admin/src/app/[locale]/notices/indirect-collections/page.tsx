import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { IndirectCollectionsContent } from "./indirect-collections-content";

/** PNG-04 indirect-collection notices (ม.25, admin: /notices/indirect-collections). Gated on notice.indirect.read. */
export default async function IndirectCollectionsPage() {
  const t = await getTranslations("indirectCollections");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <IndirectCollectionsContent />
    </GrantsProvider>
  );
}
