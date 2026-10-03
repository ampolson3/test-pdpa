import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { CalendarContent } from "./calendar-content";

/** ORG-20 business calendars and holidays (admin: /settings/calendar). Gated on org.settings.read in CalendarContent. */
export default async function CalendarPage() {
  const t = await getTranslations("calendar");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <CalendarContent />
    </GrantsProvider>
  );
}
