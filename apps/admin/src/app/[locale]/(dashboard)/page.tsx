import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import type { components } from "@pdpa/api-client";
import { apiFetch } from "@/lib/api";
import { signIn } from "@/auth";
import { DashboardContent } from "./dashboard-content";

type Me = components["schemas"]["Me"];

export default async function DashboardPage() {
  const t = await getTranslations("dashboard");

  let me: Me;
  try {
    const res = await apiFetch("/admin/v1/me");
    if (!res.ok) throw res;
    me = await res.json();
  } catch {
    return (
      <main className="flex min-h-screen items-center justify-center">
        <form
          action={async () => {
            "use server";
            await signIn("keycloak");
          }}
        >
          <button type="submit" className="rounded-md bg-slate-900 px-4 py-2 text-white">
            {t("signIn")}
          </button>
        </form>
      </main>
    );
  }

  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DashboardContent me={me} />
    </GrantsProvider>
  );
}
