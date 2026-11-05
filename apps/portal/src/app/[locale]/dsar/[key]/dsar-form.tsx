"use client";

import { useActionState } from "react";
import { useTranslations } from "next-intl";
import type { DsarPublicIntakeForm } from "@/lib/dsar";
import { submitDsarRequest, confirmOtp, type SubmitState } from "./actions";

export function DsarForm({ form, publicKey, locale, idem }: { form: DsarPublicIntakeForm; publicKey: string; locale: string; idem: string }) {
  const t = useTranslations("portal.dsar");
  const [state, action, pending] = useActionState(submitDsarRequest.bind(null, publicKey, locale), { idem } as SubmitState);

  if (state.requestNo) {
    return <ConfirmPanel publicKey={publicKey} locale={locale} state={state} />;
  }

  return (
    <form action={action} className="space-y-5 rounded-lg bg-white p-6 shadow-sm">
      <header>
        <h1 className="text-lg font-semibold">{form.name}</h1>
        <p className="text-sm text-slate-600">{t("intro")}</p>
      </header>

      <label className="block text-sm">
        {t("requestType")}
        <select name="request_type_code" required className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="request-type">
          <option value="">{t("requestTypePlaceholder")}</option>
          {form.request_types.map((rt) => (
            <option key={rt.code} value={rt.code}>
              {locale === "en" && rt.name_en ? rt.name_en : rt.name_th}
            </option>
          ))}
        </select>
      </label>

      <label className="block text-sm">
        {t("name")}
        <input name="requester_name" required className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="requester-name" />
      </label>

      <fieldset className="space-y-2">
        <legend className="font-medium">{t("contact")}</legend>
        <p className="text-sm text-slate-600">{t("contactHint")}</p>
        <label className="block text-sm">
          {t("email")}
          <input type="email" name="requester_email" autoComplete="email" className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="requester-email" />
        </label>
        <label className="block text-sm">
          {t("phone")}
          <input type="tel" name="requester_phone" autoComplete="tel" className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="requester-phone" />
        </label>
      </fieldset>

      <label className="block text-sm">
        {t("dataSource")}
        <textarea name="data_source" className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" rows={2} />
        <span className="text-xs text-slate-500">{t("dataSourceHint")}</span>
      </label>

      {state.error && (
        <p role="alert" className="text-sm text-rose-700" data-testid="error">
          {t.has(`errors.${state.error.code.replace(".", "_")}`) ? t(`errors.${state.error.code.replace(".", "_")}`) : t("errors.generic")}
        </p>
      )}

      <button type="submit" disabled={pending} className="rounded-md bg-blue-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-60" data-testid="submit">
        {pending ? t("submitting") : t("submit")}
      </button>
    </form>
  );
}

function ConfirmPanel({ publicKey, locale, state }: { publicKey: string; locale: string; state: SubmitState }) {
  const t = useTranslations("portal.dsar");
  const v = state.verification;
  const boundConfirm = v ? confirmOtp.bind(null, publicKey, v.request_id, v.id, locale) : null;
  const [confirmState, confirmAction, pending] = useActionState(boundConfirm ?? (async (s: SubmitState, _form: FormData) => s), state);

  return (
    <section className="space-y-3 rounded-lg bg-white p-6 shadow-sm" role="status" data-testid="dsar-submitted">
      <h1 className="text-lg font-semibold">{t("thanks")}</h1>
      <p>{t("requestNo", { requestNo: state.requestNo! })}</p>

      {confirmState.confirmed ? (
        <p className="text-sm text-emerald-700" data-testid="otp-confirmed">
          {t("otpConfirmed")}
        </p>
      ) : v ? (
        <form action={confirmAction} className="space-y-2 border-t border-slate-200 pt-3">
          <label className="block text-sm">
            {t("otpLabel")}
            <input name="code" required inputMode="numeric" maxLength={20} className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="otp-code" />
          </label>
          {confirmState.error && (
            <p role="alert" className="text-sm text-rose-700">
              {t.has(`errors.${confirmState.error.code.replace(".", "_")}`) ? t(`errors.${confirmState.error.code.replace(".", "_")}`) : t("errors.generic")}
            </p>
          )}
          <button type="submit" disabled={pending} className="rounded-md bg-blue-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-60" data-testid="otp-submit">
            {pending ? t("submitting") : t("otpConfirm")}
          </button>
        </form>
      ) : null}
    </section>
  );
}
