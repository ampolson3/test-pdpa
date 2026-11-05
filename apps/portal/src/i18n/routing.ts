import { defineRouting } from "next-intl/routing";
import { createNavigation } from "next-intl/navigation";
import { defaultLocale, locales } from "@pdpa/i18n";

export const routing = defineRouting({
  locales,
  defaultLocale,
});

export const { Link, usePathname, useRouter, redirect } = createNavigation(routing);
