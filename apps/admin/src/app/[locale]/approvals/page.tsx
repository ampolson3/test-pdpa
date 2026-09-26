import { getTranslations } from "next-intl/server";
import { loadMe } from "@/lib/me";
import { ApprovalsContent } from "./approvals-content";

/** PLT-08 approval inbox (admin: /approvals): versions waiting on the signed-in user's decision. */
export default async function ApprovalsPage() {
  const t = await getTranslations("versions.inbox");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return <ApprovalsContent />;
}
