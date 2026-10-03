"use client";

import { useLocale, useTranslations } from "next-intl";
import { Can } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import type { components } from "@pdpa/api-client";
import { Link } from "@/i18n/routing";

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

      <Can permission="breach.incident.read">
        <section>
          <h2 className="text-sm font-medium text-slate-500">{t("breach")}</h2>
          <Link className="text-sm text-sky-700 underline" href="/incidents">{t("breachRegister")}</Link>
        </section>
      </Can>

      <section>
        <h2 className="text-sm font-medium text-slate-500">{t("documents")}</h2>
        <Link className="text-sm text-sky-700 underline" href="/documents">{t("documents")}</Link>
      </section>

      <Can permission="consent.record.read">
        <ConsentLinks />
      </Can>

      {/* Example of a permission-gated action — real ones land as each feature is implemented. */}
      <Can permission="iam.user.create">
        <Button variant="secondary">iam.user.create</Button>
      </Can>
    </main>
  );
}

function ConsentLinks() {
  const t = useTranslations("consent.nav");
  return (
    <section>
      <h2 className="text-sm font-medium text-slate-500">{t("title")}</h2>
      <ul className="mt-1 flex flex-wrap gap-3 text-sm">
        <li><Link className="text-sky-700 underline" href="/consent/purposes">{t("purposes")}</Link></li>
        <li><Link className="text-sky-700 underline" href="/consent/collection-points">{t("collectionPoints")}</Link></li>
        <li><Link className="text-sky-700 underline" href="/consent/subjects">{t("subjects")}</Link></li>
      </ul>
    </section>
  );
}
