import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { IncidentContent } from "./incident-content";

/** One incident (admin: /incidents/{id}): clock, lifecycle, assessment, decision, timeline, evidence, notices. */
export default async function IncidentPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const t = await getTranslations("breach");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <IncidentContent id={id} meId={me.id} />
    </GrantsProvider>
  );
}
