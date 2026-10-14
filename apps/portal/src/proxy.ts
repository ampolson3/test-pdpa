import type { NextRequest } from "next/server";
import createMiddleware from "next-intl/middleware";
import { routing } from "@/i18n/routing";

const intl = createMiddleware(routing);

// PLT-17's own "CSP แบบเข้ม" (docs/architecture/security.md): a strict, per-request nonce CSP on
// every portal response. The nonce is set on the *request* headers before next-intl runs — next-intl
// builds its own downstream request headers from `new Headers(request.headers)`
// (next-intl/dist/.../middleware.js), so whatever is set here survives into the request Next.js
// renders Server Components against, and the App Router automatically tags its own hydration
// scripts with a nonce it finds there. style-src keeps 'unsafe-inline': Next/Tailwind's own injected
// <style> tags have no nonce support, and inline style injection is a far narrower XSS surface than
// inline script, which is why only script-src is strict here.
export default function proxy(request: NextRequest) {
  const nonce = Buffer.from(crypto.randomUUID()).toString("base64");
  const csp = [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'`,
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data:",
    "font-src 'self'",
    "connect-src 'self'",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
    "upgrade-insecure-requests",
  ].join("; ");

  request.headers.set("x-nonce", nonce);

  const response = intl(request);
  response.headers.set("Content-Security-Policy", csp);
  response.headers.set("X-Frame-Options", "DENY");
  response.headers.set("X-Content-Type-Options", "nosniff");
  response.headers.set("Referrer-Policy", "strict-origin-when-cross-origin");
  return response;
}

export const config = {
  matcher: ["/((?!api|_next|.*\\..*).*)"],
};
