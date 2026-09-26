import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { JobsContent } from "./jobs-content";

/** PLT-10 job-status page (admin: /settings/jobs). Gated on admin.job.read in JobsContent. */
export default async function JobsPage() {
  const t = await getTranslations("jobs");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <JobsContent />
    </GrantsProvider>
  );
}
