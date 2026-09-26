import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { ActivityDetailContent } from "./activity-detail-content";

/** One RoPA processing activity (admin: /ropa/activities/{id}): ม.39 fields, purposes, data, retention, recipients, submit. */
export default async function ActivityDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const t = await getTranslations("activities");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <ActivityDetailContent id={id} />
    </GrantsProvider>
  );
}
