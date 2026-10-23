"use client";

import { useActionState } from "react";
import { useTranslations } from "next-intl";
import type { PreferenceCenter } from "@/lib/preferences";
import { requestCode, confirmCode, savePreferences, type StartState, type ViewState, type SaveState } from "./actions";

export function PreferenceCenterForm({ publicKey, locale, idem }: { publicKey: string; locale: string; idem: string }) {
  const t = useTranslations("portal.preferences");
  const [start, startAction, startPending] = useActionState(requestCode.bind(null, publicKey, locale), { idem } as StartState);

  if (!start.verificationId) {
    return (
      <form action={startAction} className="space-y-5 rounded-lg bg-white p-6 shadow-sm">
        <header>
          <h1 className="text-lg font-semibold">{t("title")}</h1>
          <p className="text-sm text-slate-600">{t("intro")}</p>
        </header>
        <fieldset className="space-y-2">
          <legend className="font-medium">{t("contact")}</legend>
          <label className="block text-sm">
            {t("email")}
            <input type="email" name="email" autoComplete="email" className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="email" />
          </label>
          <label className="block text-sm">
            {t("phone")}
            <input type="tel" name="phone" autoComplete="tel" className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" />
          </label>
        </fieldset>
        {start.error && (
          <p className="text-sm text-red-700" role="alert" data-testid="start-error">
            {t.has(`errors.${start.error}`) ? t(`errors.${start.error}`) : t("errors.generic")}
          </p>
        )}
        <button type="submit" disabled={startPending} className="rounded-md bg-slate-900 px-4 py-2 text-white disabled:opacity-50" data-testid="send-code">
          {startPending ? t("sending") : t("sendCode")}
        </button>
      </form>
    );
  }

  return <VerifyAndEdit publicKey={publicKey} locale={locale} verificationId={start.verificationId} channel={start.channel ?? "email"} identifier={start.identifier!} />;
}

function VerifyAndEdit({
  publicKey,
  locale,
  verificationId,
  channel,
  identifier,
}: {
  publicKey: string;
  locale: string;
  verificationId: string;
  channel: string;
  identifier: { type: string; value: string };
}) {
  const t = useTranslations("portal.preferences");
  const [view, viewAction, viewPending] = useActionState(confirmCode.bind(null, publicKey, locale, verificationId, identifier), { idem: crypto.randomUUID() } as ViewState);

  if (!view.center) {
    return (
      <form action={viewAction} className="space-y-5 rounded-lg bg-white p-6 shadow-sm">
        <header>
          <h1 className="text-lg font-semibold">{t("verifyTitle")}</h1>
          <p className="text-sm text-slate-600">{t(`codeSentVia.${channel}`)}</p>
        </header>
        <label className="block text-sm">
          {t("code")}
          <input type="text" inputMode="numeric" name="code" autoComplete="one-time-code" required className="mt-0.5 w-full rounded-md border border-slate-300 px-2 py-1.5" data-testid="code" />
        </label>
        {view.error && (
          <p className="text-sm text-red-700" role="alert" data-testid="verify-error">
            {t.has(`errors.${view.error}`) ? t(`errors.${view.error}`) : t("errors.generic")}
          </p>
        )}
        <button type="submit" disabled={viewPending} className="rounded-md bg-slate-900 px-4 py-2 text-white disabled:opacity-50" data-testid="verify-code">
          {viewPending ? t("sending") : t("verify")}
        </button>
      </form>
    );
  }

  return <PreferenceList publicKey={publicKey} locale={locale} verificationId={verificationId} identifier={identifier} center={view.center} />;
}

function PreferenceList({
  publicKey,
  locale,
  verificationId,
  identifier,
  center,
}: {
  publicKey: string;
  locale: string;
  verificationId: string;
  identifier: { type: string; value: string };
  center: PreferenceCenter;
}) {
  const t = useTranslations("portal.preferences");
  const [state, action, pending] = useActionState(savePreferences.bind(null, publicKey, locale, verificationId, identifier, center), { idem: crypto.randomUUID() } as SaveState);

  if (state.receipt) {
    return (
      <section className="space-y-2 rounded-lg bg-white p-6 shadow-sm" role="status" data-testid="preferences-saved">
        <h1 className="text-lg font-semibold">{t("saved")}</h1>
        <p>{t("receipt", { receipt: state.receipt })}</p>
      </section>
    );
  }

  return (
    <form action={action} className="space-y-5 rounded-lg bg-white p-6 shadow-sm">
      <header>
        <h1 className="text-lg font-semibold">{t("title")}</h1>
        <p className="text-sm text-slate-600">{t("editIntro")}</p>
      </header>
      <ul className="space-y-4">
        {center.purposes.map((p) => (
          <li key={p.code} className={`space-y-2 rounded-md border p-4 ${p.is_sensitive ? "border-rose-200" : "border-slate-200"}`} data-testid={`purpose-${p.code}`}>
            <h2 className="font-medium">{p.name}</h2>
            {p.description && <p className="text-sm text-slate-600">{p.description}</p>}
            <p className="text-xs text-slate-500">
              {t("currentStatus")}: {t.has(`status.${p.decision ?? "NONE"}`) ? t(`status.${p.decision ?? "NONE"}`) : p.decision ?? t("status.NONE")}
            </p>
            <fieldset className="flex flex-wrap gap-4 text-sm">
              <label className="flex items-center gap-1">
                <input type="radio" name={`decision:${p.code}`} value="keep" defaultChecked /> {t("keep")}
              </label>
              <label className="flex items-center gap-1">
                <input type="radio" name={`decision:${p.code}`} value="grant" data-testid={`grant-${p.code}`} /> {t("grant")}
              </label>
              {p.decision === "ACTIVE" && (
                <label className="flex items-center gap-1">
                  <input type="radio" name={`decision:${p.code}`} value="withdraw" data-testid={`withdraw-${p.code}`} /> {t("withdraw")}
                </label>
              )}
            </fieldset>
          </li>
        ))}
      </ul>
      {state.error && (
        <p className="text-sm text-red-700" role="alert" data-testid="save-error">
          {t.has(`errors.${state.error}`) ? t(`errors.${state.error}`) : t("errors.generic")}
        </p>
      )}
      <button type="submit" disabled={pending} className="rounded-md bg-slate-900 px-4 py-2 text-white disabled:opacity-50" data-testid="save">
        {pending ? t("sending") : t("save")}
      </button>
    </form>
  );
}
