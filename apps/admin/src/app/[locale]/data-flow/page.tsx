import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DataFlowContent } from "./data-flow-content";

/** DFG-01/DFG-03: the data flow diagram generated live from RoPA (admin: /data-flow). Gated on
 *  ropa.dataflow.read in DataFlowContent. */
export default async function DataFlowPage() {
  const t = await getTranslations("dataflow");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-6xl p-8 text-slate-600">{t("forbidden")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DataFlowContent />
    </GrantsProvider>
  );
}
