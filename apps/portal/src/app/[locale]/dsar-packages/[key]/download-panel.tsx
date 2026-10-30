"use client";

import { useActionState } from "react";
import { useTranslations } from "next-intl";
import type { DsarPublicPackage } from "@/lib/dsar-package";
import { downloadPackage, type DownloadState } from "./actions";

export function DownloadPanel({ pkg, publicKey, locale }: { pkg: DsarPublicPackage; publicKey: string; locale: string }) {
  const t = useTranslations("portal.dsarPackage");
  const [state, action, pending] = useActionState(downloadPackage.bind(null, publicKey, locale), {} as DownloadState);

  if (state.downloadUrl) {
    return (
      <section className="space-y-3 rounded-lg bg-white p-6 shadow-sm" role="status" data-testid="package-download-ready">
        <h1 className="text-lg font-semibold">{t("downloadReady")}</h1>
        <a href={state.downloadUrl} className="inline-block rounded-md bg-blue-700 px-4 py-2 text-sm font-medium text-white" data-testid="download-link">
          {t("downloadLink")}
        </a>
      </section>
    );
  }

  return (
    <form action={action} className="space-y-5 rounded-lg bg-white p-6 shadow-sm">
      <header>
        <h1 className="text-lg font-semibold">{t("title")}</h1>
        <p className="text-sm text-slate-600">{t("expiresAt", { expiresAt: new Date(pkg.expires_at).toLocaleString(locale) })}</p>
      </header>

      {pkg.password_protected && (
        <label className="block text-sm">
          {t("accessCodeLabel")}
          <input name="access_code" required className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5 font-mono uppercase" data-testid="access-code" />
        </label>
      )}

      {state.error && (
        <p role="alert" className="text-sm text-rose-700" data-testid="error">
          {t.has(`errors.${state.error.code.replace(".", "_")}`) ? t(`errors.${state.error.code.replace(".", "_")}`) : t("errors.generic")}
        </p>
      )}

      <button type="submit" disabled={pending} className="rounded-md bg-blue-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-60" data-testid="download-submit">
        {pending ? t("downloading") : t("download")}
      </button>
    </form>
  );
}
