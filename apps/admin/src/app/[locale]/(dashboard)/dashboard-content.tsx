"use client";

import { useLocale, useTranslations } from "next-intl";
import { Can } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import type { components } from "@pdpa/api-client";

type Me = components["schemas"]["Me"];

export function DashboardContent({ me }: { me: Me }) {
  const t = useTranslations("dashboard");
  const locale = useLocale() as Locale;

  return (
    <main className="mx-auto max-w-2xl space-y-4 p-8">
      <h1 className="text-xl font-semibold">{t("welcome", { name: me.display_name })}</h1>
      <p className="text-slate-600">{t("tenant", { name: me.tenant.name })}</p>
      {/* Buddhist Era for th, Gregorian for en, both in Asia/Bangkok (CLAUDE.md rule 11) */}
      <p className="text-sm text-slate-500">{t("today", { date: formatDate(new Date().toISOString(), locale) })}</p>

      <section>
        <h2 className="text-sm font-medium text-slate-500">{t("roles")}</h2>
        <ul className="mt-1 flex flex-wrap gap-2">
          {me.roles.map((role) => (
            <li key={role} className="rounded-full bg-slate-200 px-3 py-1 text-xs">
              {role}
            </li>
          ))}
        </ul>
      </section>

      {/* Example of a permission-gated action — real ones land as each feature is implemented. */}
      <Can permission="iam.user.create">
        <Button variant="secondary">iam.user.create</Button>
      </Can>
    </main>
  );
}
