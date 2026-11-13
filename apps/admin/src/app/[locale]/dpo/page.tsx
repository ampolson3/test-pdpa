import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DeadlinesContent } from "./deadlines-content";

/** DPO-05 notification center (admin: /dpo). Gated on dpo.report.read in DeadlinesContent. */
export default async function DpoDeadlinesPage() {
  const t = await getTranslations("dpoDeadlines");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DeadlinesContent />
    </GrantsProvider>
  );
}
