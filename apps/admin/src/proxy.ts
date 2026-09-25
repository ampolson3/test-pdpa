import createMiddleware from "next-intl/middleware";
import { defaultLocale, locales } from "@pdpa/i18n";

/**
 * Locale routing only for now (th default, en). The auth gate (redirect to Keycloak login when
 * there's no session) belongs here too once IAM-01/02 land — Auth.js v5's `auth` export can wrap
 * this same middleware — left out until there's a login flow to test it against.
 */
export default createMiddleware({
  locales,
  defaultLocale,
});

export const config = {
  matcher: ["/((?!api|_next|.*\\..*).*)"],
};
