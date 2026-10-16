import { getTranslations } from "next-intl/server";
import { loadMe } from "@/lib/me";
import { ResponseContent } from "./response-content";

/** PLT-06 response (admin: /form-responses/{id}): answer, assign sections, submit, see the score. */
export default async function FormResponsePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const t = await getTranslations("forms");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return <ResponseContent id={id} />;
}
