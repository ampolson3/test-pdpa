import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { QuestionnairesContent } from "./questionnaires-content";

/** ROPA-11 data mapping questionnaire (admin: /ropa/questionnaires). Gated on ropa.activity.read in
 *  QuestionnairesContent — send/approve/reject are gated on their own permissions within that component. */
export default async function RopaQuestionnairesPage() {
  const t = await getTranslations("ropaQuestionnaires");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <QuestionnairesContent />
    </GrantsProvider>
  );
}
