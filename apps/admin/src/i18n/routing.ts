import { defineRouting } from "next-intl/routing";
import { createNavigation } from "next-intl/navigation";
import { defaultLocale, locales } from "@pdpa/i18n";

export const routing = defineRouting({
  locales,
  defaultLocale,
});

// Locale-aware Link/usePathname/useRouter — the LocaleSwitcher (PLT-03) uses these to switch
// language while staying on the current page, instead of hand-rolling path rewriting.
export const { Link, usePathname, useRouter, redirect } = createNavigation(routing);
