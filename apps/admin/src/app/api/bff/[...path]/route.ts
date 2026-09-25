import { NextRequest, NextResponse } from "next/server";
import { apiFetch } from "@/lib/api";

/**
 * BFF proxy for client components: attaches the Keycloak access token server-side and forwards to
 * the Go API (docs/architecture/code-structure.md, Frontend: "app/api/bff/[...path] — proxy ไป Go
 * API แนบ JWT ฝั่ง server"). Client components call this path, never the Go API directly, so the
 * token never reaches the browser. Server Components skip this hop and call lib/api's apiFetch
 * directly. CSRF checking for state-changing methods is still open — add it here before this proxy
 * handles anything beyond the read-only reference slice.
 */
async function proxy(req: NextRequest, path: string[]): Promise<NextResponse> {
  let upstream: Response;
  try {
    upstream = await apiFetch(`/${path.join("/")}${req.nextUrl.search}`, {
      method: req.method,
      headers: {
        "Content-Type": req.headers.get("content-type") ?? "application/json",
        "Accept-Language": req.headers.get("accept-language") ?? "th",
      },
      body: ["GET", "HEAD"].includes(req.method) ? undefined : await req.text(),
    });
  } catch {
    return NextResponse.json({ code: "authn.required", title: "Authentication required", status: 401 }, { status: 401 });
  }

  const body = await upstream.text();
  return new NextResponse(body, {
    status: upstream.status,
    headers: { "Content-Type": upstream.headers.get("content-type") ?? "application/json" },
  });
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
