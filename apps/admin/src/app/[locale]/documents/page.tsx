import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DocumentsContent } from "./documents-content";

/** PLT-16 document composer. What the caller sees is decided by the API (per document type). */
export default async function Page() {
  const t = await getTranslations("docs");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DocumentsContent />
    </GrantsProvider>
  );
}
