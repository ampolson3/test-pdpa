import { getTranslations } from "next-intl/server";

// Data-subject / guest portal (docs/architecture/code-structure.md: /preferences, /c/[id], /request, ...).
// Only the hosted consent form (/c/[key], CON-09) exists so far; pages are reached by the links organizations share.
export default async function PortalHome() {
  const t = await getTranslations("portal");
  return (
    <main className="flex min-h-screen items-center justify-center">
      <p className="text-slate-500">{t("home")}</p>
    </main>
  );
}
