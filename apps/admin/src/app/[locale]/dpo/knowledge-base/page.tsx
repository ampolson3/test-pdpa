import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { KnowledgeBaseContent } from "./knowledge-base-content";

/** DPO-12: legal documents and FAQ knowledge base (admin: /dpo/knowledge-base). */
export default async function KnowledgeBasePage() {
  const t = await getTranslations("knowledgeBase");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <KnowledgeBaseContent />
    </GrantsProvider>
  );
}
