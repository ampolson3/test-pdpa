import createMiddleware from "next-intl/middleware";
import { routing } from "@/i18n/routing";

/**
 * Locale routing only for now (th default, en). The auth gate (redirect to Keycloak login when
 * there's no session) belongs here too once IAM-01/02 land — Auth.js v5's `auth` export can wrap
 * this same middleware — left out until there's a login flow to test it against.
 */
export default createMiddleware(routing);

export const config = {
  matcher: ["/((?!api|_next|.*\\..*).*)"],
};
