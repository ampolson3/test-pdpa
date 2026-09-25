import { NextRequest, NextResponse } from "next/server";
import { locales, defaultLocale } from "@pdpa/i18n";
import { apiFetch } from "@/lib/api";

/**
 * BFF proxy for client components: attaches the Keycloak access token server-side and forwards to
 * the Go API (docs/architecture/code-structure.md, Frontend: "app/api/bff/[...path] — proxy ไป Go
 * API แนบ JWT ฝั่ง server"). Client components call this path, never the Go API directly, so the
 * token never reaches the browser. Server Components skip this hop and call lib/api's apiFetch
 * directly.
 *
 * - CSRF: a state-changing request must come from this app's own origin (Origin header, which
 *   browsers always send on cross-origin and non-GET requests); anything else is refused before the
 *   session's token is attached.
 * - Bodies are streamed through untouched, so binary uploads (multipart, PLT-09) arrive intact.
 * - Responses are streamed back (server-sent events for the inbox, PLT-04), with ETag for If-Match.
 * - Redirects are passed back to the browser, not followed: a file download answers 302 to a
 *   short-lived signed object-storage URL the browser should fetch directly.
 */
async function proxy(req: NextRequest, path: string[]): Promise<NextResponse> {
  const mutating = !["GET", "HEAD"].includes(req.method);
  if (mutating && !sameOrigin(req)) {
    return NextResponse.json({ code: "authz.denied", title: "Cross-origin request refused", status: 403 }, { status: 403 });
  }

  let upstream: Response;
  try {
    upstream = await apiFetch(`/${path.join("/")}${req.nextUrl.search}`, {
      method: req.method,
      headers: {
        "Content-Type": req.headers.get("content-type") ?? "application/json",
        "Accept-Language": localeFromReferer(req),
        ...(req.headers.get("if-match") ? { "If-Match": req.headers.get("if-match")! } : {}),
      },
      body: mutating ? req.body : undefined,
      // Node's fetch requires this to send a streamed request body.
      ...(mutating ? { duplex: "half" } : {}),
      redirect: "manual",
    } as RequestInit);
  } catch {
    return NextResponse.json({ code: "authn.required", title: "Authentication required", status: 401 }, { status: 401 });
  }

  const location = upstream.headers.get("location");
  if (upstream.status >= 300 && upstream.status < 400 && location) {
    return new NextResponse(null, { status: upstream.status, headers: { Location: location } });
  }

  // Streamed, not buffered: the inbox's server-sent events (PLT-04) never end on their own.
  const headers: Record<string, string> = { "Content-Type": upstream.headers.get("content-type") ?? "application/json" };
  for (const h of ["etag", "cache-control"]) {
    const v = upstream.headers.get(h);
    if (v) headers[h] = v;
  }
  return new NextResponse(upstream.body, { status: upstream.status, headers });
}

function sameOrigin(req: NextRequest): boolean {
  const origin = req.headers.get("origin");
  return origin !== null && origin === req.nextUrl.origin;
}

/**
 * This route lives under app/api/, outside the app/[locale]/ segment, so next-intl has no route
 * param to resolve a locale from here the way it does on a page. The browser's Referer reliably
 * carries the page's own path (/th/... or /en/...), which is what we actually want: the locale the
 * user is looking at, not their browser's Accept-Language preference list (PLT-03 — see lib/api.ts
 * for why the raw header can't be forwarded as-is either).
 */
function localeFromReferer(req: NextRequest): string {
  const referer = req.headers.get("referer");
  if (!referer) return defaultLocale;
  const segment = new URL(referer).pathname.split("/")[1];
  return (locales as readonly string[]).includes(segment) ? segment : defaultLocale;
}

export async function GET(req: NextRequest, { params }: { params: Promise<{ path: string[] }> }) {
  return proxy(req, (await params).path);
}
export async function POST(req: NextRequest, { params }: { params: Promise<{ path: string[] }> }) {
  return proxy(req, (await params).path);
}
export async function PATCH(req: NextRequest, { params }: { params: Promise<{ path: string[] }> }) {
  return proxy(req, (await params).path);
}
export async function PUT(req: NextRequest, { params }: { params: Promise<{ path: string[] }> }) {
  return proxy(req, (await params).path);
}
export async function DELETE(req: NextRequest, { params }: { params: Promise<{ path: string[] }> }) {
  return proxy(req, (await params).path);
}
