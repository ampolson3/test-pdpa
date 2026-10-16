import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { VendorDetailContent } from "./vendor-detail-content";

/** One vendor's full profile (admin: /vendors/{id}) — VEN-01's own "single page" acceptance criterion. */
export default async function VendorDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const t = await getTranslations("vendors");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <VendorDetailContent id={id} />
    </GrantsProvider>
  );
}
