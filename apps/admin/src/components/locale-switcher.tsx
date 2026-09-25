"use client";

import { useLocale, useTranslations } from "next-intl";
import { locales } from "@pdpa/i18n";
import { usePathname, useRouter } from "@/i18n/routing";

const LABELS: Record<(typeof locales)[number], string> = {
  th: "ไทย",
  en: "English",
};

/** PLT-03 acceptance criterion: every page can switch TH/EN. Stays on the current page. */
export function LocaleSwitcher() {
  const locale = useLocale();
  const pathname = usePathname();
  const router = useRouter();
  const t = useTranslations("common");

  return (
    <select
      aria-label={t("language")}
      value={locale}
      onChange={(e) => router.replace(pathname, { locale: e.target.value })}
      className="rounded-md border border-slate-300 bg-white px-2 py-1 text-sm"
    >
      {locales.map((l) => (
        <option key={l} value={l}>
          {LABELS[l]}
        </option>
      ))}
    </select>
  );
}
