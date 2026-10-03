import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DeliveriesContent } from "./deliveries-content";

/** PLT-04 delivery status (admin: /settings/notifications). */
export default async function NotificationsPage() {
  const t = await getTranslations("notify");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DeliveriesContent />
    </GrantsProvider>
  );
}
