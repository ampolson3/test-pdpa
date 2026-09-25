import createMiddleware from "next-intl/middleware";
import { routing } from "@/i18n/routing";

/** Locale routing (th default, en). The portal has no sign-in: pages are reached by public links. */
export default createMiddleware(routing);

export const config = {
  matcher: ["/((?!api|_next|.*\\..*).*)"],
};
