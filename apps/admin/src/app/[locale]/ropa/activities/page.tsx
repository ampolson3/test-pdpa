import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { ActivitiesContent } from "./activities-content";

/** ROPA-03 processing activity register (admin: /ropa/activities). Gated on ropa.activity.read in ActivitiesContent. */
export default async function ActivitiesPage() {
  const t = await getTranslations("activities");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <ActivitiesContent />
    </GrantsProvider>
  );
}
