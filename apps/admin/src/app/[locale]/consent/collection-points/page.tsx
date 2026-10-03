import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { CollectionPointsContent } from "./collection-points-content";

/** CON-09/10 collection points (admin: /consent/collection-points). Gated on consent.collectionpoint.read in CollectionPointsContent. */
export default async function CollectionPointsPage() {
  const t = await getTranslations("consent");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <CollectionPointsContent portalUrl={process.env.PORTAL_PUBLIC_URL ?? "http://localhost:3001"} />
    </GrantsProvider>
  );
}
