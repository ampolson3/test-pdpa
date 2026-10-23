import { getTranslations } from "next-intl/server";
import { loadMe } from "@/lib/me";
import { FormBuilder } from "./form-builder";

/** PLT-06 form builder (admin: /forms/{id}): edit the draft, preview in th/en, publish, see responses. */
export default async function FormPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const t = await getTranslations("forms");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return <FormBuilder id={id} />;
}
