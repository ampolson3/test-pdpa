"use client";

import { useActionState } from "react";
import { useTranslations } from "next-intl";
import type { PublicCollectionPoint } from "@/lib/consent";
import { submitConsent, type SubmitState } from "./actions";

export function ConsentForm({ cp, publicKey, locale, idem }: { cp: PublicCollectionPoint; publicKey: string; locale: string; idem: string }) {
  const t = useTranslations("portal.consent");
  const [state, action, pending] = useActionState(submitConsent.bind(null, publicKey, locale), { idem } as SubmitState);

  if (state.receipt) {
    return (
      <section className="space-y-2 rounded-lg bg-white p-6 shadow-sm" role="status" data-testid="consent-done">
        <h1 className="text-lg font-semibold">{t("thanks")}</h1>
        <p>{t("receipt", { receipt: state.receipt })}</p>
        <p className="text-sm text-slate-600">{t("withdrawInfo")}</p>
      </section>
    );
  }
  const fieldError = (code: string) => state.error?.fields.find((f) => f.field === code);

  return (
    <form action={action} className="space-y-5 rounded-lg bg-white p-6 shadow-sm">
      <header>
        <h1 className="text-lg font-semibold">{cp.name}</h1>
        <p className="text-sm text-slate-600">{t("intro")}</p>
      </header>

      <fieldset className="space-y-2">
        <legend className="font-medium">{t("contact")}</legend>
        <p className="text-sm text-slate-600">{t("contactHint")}</p>
        <label className="block text-sm">
          {t("email")}
          <input type="email" name="email" autoComplete="email" className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="email" />
        </label>
        <label className="block text-sm">
          {t("phone")}
          <input type="tel" name="phone" autoComplete="tel" className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" />
        </label>
      </fieldset>

      <ul className="space-y-4">
        {cp.purposes.map((p) => {
          const err = fieldError(p.code);
          return (
            <li key={p.code} className={`space-y-2 rounded-md border p-4 ${p.is_sensitive ? "border-rose-200" : "border-slate-200"}`} data-testid={`purpose-${p.code}`}>
              <input type="hidden" name={`version:${p.code}`} value={p.version_no} />
              <h2 className="font-medium">
                {p.name}
                {p.required && <span className="ml-2 text-xs text-slate-500">{t("required")}</span>}
              </h2>
              {p.description && <p className="text-sm text-slate-600">{p.description}</p>}
              <p className="text-sm whitespace-pre-line">{p.is_sensitive && p.explicit_text ? p.explicit_text : p.text}</p>
              {p.min_age ? <p className="text-xs text-slate-500">{t("minAge", { age: p.min_age })}</p> : null}
              <label className="flex items-start gap-2">
                <input type="checkbox" name={`consent:${p.code}`} className="mt-1" required={!!p.required} data-testid={`consent-${p.code}`} />
                <span>{p.is_sensitive ? t("agreeExplicit") : t("agree")}</span>
              </label>
              {p.preferences?.map((pref) => (
                <fieldset key={pref.code} className="ml-6 space-y-1 text-sm">
                  <legend className="text-slate-600">{pref.name}</legend>
                  <div className="flex flex-wrap gap-3">
                    {pref.options?.map((o) => (
                      <label key={o.value} className="flex items-center gap-1">
                        <input type="checkbox" name={`pref:${p.code}:${pref.code}`} value={o.value} defaultChecked />
                        {o.label}
                      </label>
                    ))}
                  </div>
                </fieldset>
              ))}
              {err && <p className="text-sm text-red-700" role="alert">{t.has(`errors.${err.code}`) ? t(`errors.${err.code}`) : t("errors.generic")}</p>}
            </li>
          );
        })}
      </ul>

      {state.error && (
        <p className="text-sm text-red-700" role="alert" data-testid="consent-error">
          {t.has(`errors.${state.error.code.replace(".", "_")}`) ? t(`errors.${state.error.code.replace(".", "_")}`) : t("errors.generic")}
        </p>
      )}
      <p className="text-xs text-slate-500">{t("withdrawInfo")}</p>
      <button type="submit" disabled={pending} className="rounded-md bg-slate-900 px-4 py-2 text-white disabled:opacity-50" data-testid="submit">
        {pending ? t("sending") : t("submit")}
      </button>
    </form>
  );
}
