import { getTranslations } from "next-intl/server";
import { loadMe } from "@/lib/me";
import { FormsContent } from "./forms-content";

/** PLT-06 forms (admin: /forms): the forms of the types the user may read, and sections waiting for them. */
export default async function FormsPage() {
  const t = await getTranslations("forms");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return <FormsContent />;
}
