import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import type { components } from "@pdpa/api-client";
import { apiFetch } from "@/lib/api";
import { JobsContent } from "./jobs-content";

type Me = components["schemas"]["Me"];

/** PLT-10 job-status page (admin: /settings/jobs). Gated on admin.job.read in JobsContent. */
export default async function JobsPage() {
  const t = await getTranslations("jobs");

  let me: Me;
  try {
    const res = await apiFetch("/admin/v1/me");
    if (!res.ok) throw res;
    me = await res.json();
  } catch {
    return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  }

  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <JobsContent />
    </GrantsProvider>
  );
}
