"use client";

import { useActionState } from "react";
import { useTranslations } from "next-intl";
import { confirmDoubleOptInAction, type ConfirmState } from "./actions";

// CON-23: the subject clicks Confirm rather than the link auto-confirming on page load, so an e-mail
// link-preview scanner fetching this page never accidentally activates the consent on the subject's behalf.
export function ConfirmForm({ publicKey, locale, token, idem }: { publicKey: string; locale: string; token: string; idem: string }) {
  const t = useTranslations("portal.doubleOptIn");
  const [state, action, pending] = useActionState(confirmDoubleOptInAction.bind(null, publicKey, locale), { idem } as ConfirmState);

  if (state.purposeCode) {
    return (
      <div className="space-y-2 rounded-lg bg-white p-6 shadow-sm" role="status" data-testid="confirmed">
        <p className="text-sm text-slate-800">{t("confirmed", { purposeCode: state.purposeCode })}</p>
      </div>
    );
  }

  return (
    <form action={action} className="space-y-4 rounded-lg bg-white p-6 shadow-sm">
      <header>
        <h1 className="text-lg font-semibold">{t("title")}</h1>
        <p className="text-sm text-slate-600">{t("intro")}</p>
      </header>
      <input type="hidden" name="token" value={token} />
      {state.error && (
        <p className="text-sm text-red-700" role="alert" data-testid="confirm-error">
          {t.has(`errors.${state.error}`) ? t(`errors.${state.error}`) : t("errors.generic")}
        </p>
      )}
      <button type="submit" disabled={pending} className="rounded-md bg-slate-900 px-4 py-2 text-white disabled:opacity-50" data-testid="confirm-button">
        {pending ? t("confirming") : t("confirm")}
      </button>
    </form>
  );
}
