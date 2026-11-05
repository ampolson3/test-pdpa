"use client";

import { useActionState } from "react";
import { useTranslations } from "next-intl";
import { startTracking, trackSession, type TrackState } from "./actions";

export function TrackForm({ publicKey, locale, idem }: { publicKey: string; locale: string; idem: string }) {
  const t = useTranslations("portal.dsarTrack");
  const [state, action, pending] = useActionState(startTracking.bind(null, publicKey, locale), { idem } as TrackState);

  if (state.verificationId) {
    return <StatusPanel publicKey={publicKey} locale={locale} state={state} />;
  }

  return (
    <form action={action} className="space-y-5 rounded-lg bg-white p-6 shadow-sm">
      <header>
        <h1 className="text-lg font-semibold">{t("title")}</h1>
        <p className="text-sm text-slate-600">{t("intro")}</p>
      </header>

      <label className="block text-sm">
        {t("requestNo")}
        <input name="request_no" required className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="request-no" />
      </label>

      <label className="block text-sm">
        {t("method")}
        <select name="method" className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="method">
          <option value="otp_email">{t("methodEmail")}</option>
          <option value="otp_sms">{t("methodSms")}</option>
        </select>
      </label>

      <label className="block text-sm">
        {t("contact")}
        <input name="contact" required className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="contact" />
        <span className="text-xs text-slate-500">{t("contactHint")}</span>
      </label>

      {state.error && (
        <p role="alert" className="text-sm text-rose-700" data-testid="error">
          {t.has(`errors.${state.error.code.replace(".", "_")}`) ? t(`errors.${state.error.code.replace(".", "_")}`) : t("errors.generic")}
        </p>
      )}

      <button type="submit" disabled={pending} className="rounded-md bg-blue-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-60" data-testid="start">
        {pending ? t("submitting") : t("start")}
      </button>
    </form>
  );
}

function StatusPanel({ publicKey, locale, state }: { publicKey: string; locale: string; state: TrackState }) {
  const t = useTranslations("portal.dsarTrack");
  // One action, one state: a status check and a later document submission share the exact same evolving
  // TrackState object (two independent useActionState hooks would each start from the same stale initial
  // value and never see each other's updates) — the form's own hidden "intent" field tells the server
  // action which of the two to do.
  const [current, action, pending] = useActionState(trackSession.bind(null, publicKey, locale), state);

  return (
    <section className="space-y-5 rounded-lg bg-white p-6 shadow-sm" data-testid="tracking-session">
      <h1 className="text-lg font-semibold">{t("requestNoHeading", { requestNo: state.requestNo! })}</h1>

      {!current.status ? (
        <form action={action} className="space-y-2 border-t border-slate-200 pt-3">
          <input type="hidden" name="intent" value="check" />
          <p className="text-sm text-slate-600">{t("otpHint", { channel: t(state.channel === "sms" ? "methodSms" : "methodEmail") })}</p>
          <label className="block text-sm">
            {t("otpLabel")}
            <input name="code" required inputMode="numeric" maxLength={20} className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="otp-code" />
          </label>
          {current.error && (
            <p role="alert" className="text-sm text-rose-700" data-testid="error">
              {t.has(`errors.${current.error.code.replace(".", "_")}`) ? t(`errors.${current.error.code.replace(".", "_")}`) : t("errors.generic")}
            </p>
          )}
          <button type="submit" disabled={pending} className="rounded-md bg-blue-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-60" data-testid="otp-submit">
            {pending ? t("submitting") : t("viewStatus")}
          </button>
        </form>
      ) : (
        <div className="space-y-4" data-testid="status-view">
          <dl className="grid grid-cols-2 gap-2 text-sm">
            <dt className="text-slate-500">{t("status")}</dt>
            <dd data-testid="status-value">{t.has(`statuses.${current.status.status}`) ? t(`statuses.${current.status.status}`) : current.status.status}</dd>
            <dt className="text-slate-500">{t("dueAt")}</dt>
            <dd>{new Date(current.status.due_at).toLocaleDateString(locale)}</dd>
            {current.status.outcome && (
              <>
                <dt className="text-slate-500">{t("outcome")}</dt>
                <dd>{t.has(`outcomes.${current.status.outcome}`) ? t(`outcomes.${current.status.outcome}`) : current.status.outcome}</dd>
              </>
            )}
          </dl>

          <form action={action} className="space-y-2 border-t border-slate-200 pt-3" encType="multipart/form-data">
            <input type="hidden" name="intent" value="submit_document" />
            <p className="font-medium text-sm">{t("submitDocumentTitle")}</p>
            {current.documentSubmitted ? (
              <p className="text-sm text-emerald-700" data-testid="document-submitted">{t("documentSubmitted")}</p>
            ) : (
              <>
                <input type="file" name="file" required className="block text-sm" data-testid="document-file" />
                {current.error && (
                  <p role="alert" className="text-sm text-rose-700" data-testid="error">
                    {t.has(`errors.${current.error.code.replace(".", "_")}`) ? t(`errors.${current.error.code.replace(".", "_")}`) : t("errors.generic")}
                  </p>
                )}
                <button type="submit" disabled={pending} className="rounded-md bg-blue-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-60" data-testid="document-submit">
                  {pending ? t("submitting") : t("submitDocument")}
                </button>
              </>
            )}
          </form>
        </div>
      )}
    </section>
  );
}
