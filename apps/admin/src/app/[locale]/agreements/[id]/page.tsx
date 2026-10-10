import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { AgreementDetailContent } from "./agreement-detail-content";

/** One agreement's metadata (admin: /agreements/{id}) — DPA-02. */
export default async function AgreementDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const t = await getTranslations("agreements");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <AgreementDetailContent id={id} />
    </GrantsProvider>
  );
}
