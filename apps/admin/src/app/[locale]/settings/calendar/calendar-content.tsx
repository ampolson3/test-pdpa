"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import { ImportWizard } from "@/components/import-wizard";
import {
  createApiClient,
  useCalendars,
  useHolidayMutations,
  useHolidays,
  useOrgSettings,
  useSaveCalendar,
  useSaveOrgSettings,
  type BusinessCalendar,
  type OrgSettings,
} from "@pdpa/api-client";
import { FileUploader } from "@/components/file-uploader";

const WEEKDAYS = [1, 2, 3, 4, 5, 6, 7] as const; // ISO: 1 = Monday … 7 = Sunday
const HOLIDAY_IMPORT = "org.holiday";

type Draft = { id?: string; rowVersion?: number; name: string; timezone: string; workdays: number[]; isDefault: boolean; wasDefault: boolean };

const empty: Draft = { name: "", timezone: "Asia/Bangkok", workdays: [1, 2, 3, 4, 5], isDefault: false, wasDefault: false };

function fromCalendar(c: BusinessCalendar): Draft {
  return { id: c.id, rowVersion: c.row_version, name: c.name, timezone: c.timezone, workdays: c.workdays, isDefault: c.is_default, wasDefault: c.is_default };
}

function problemCode(e: unknown): string | undefined {
  return typeof e === "object" && e !== null && "code" in e ? String((e as { code: unknown }).code) : undefined;
}

export function CalendarContent() {
  const t = useTranslations("calendar");
  const locale = useLocale() as Locale;
  const canRead = usePermission("org.settings.read");
  const canUpdate = usePermission("org.settings.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const calendars = useCalendars(client);
  const save = useSaveCalendar(client);
  const [selected, setSelected] = useState<string>();
  const [draft, setDraft] = useState<Draft | null>(null);

  const current = calendars.data?.find((c) => c.id === selected) ?? calendars.data?.[0];
  const form = draft ?? (current ? fromCalendar(current) : null);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const submit = () => {
    if (!form) return;
    save.mutate(
      { id: form.id, rowVersion: form.rowVersion, input: { name: form.name, timezone: form.timezone, workdays: form.workdays, is_default: form.isDefault && !form.wasDefault ? true : undefined } },
      { onSuccess: (c) => { setSelected(c.id); setDraft(null); } },
    );
  };
  const saveError = save.error ? (problemCode(save.error) === "org.duplicate_calendar_name" ? t("duplicateName") : problemCode(save.error) === "org.invalid_calendar" ? t("invalid") : t("saveError")) : null;

  return (
    <main className="mx-auto max-w-6xl space-y-8 p-8">
      <GeneralSettings canUpdate={canUpdate} />
      <div className="grid gap-6 lg:grid-cols-[1fr_2fr]">
      <section className="space-y-3">
        <header className="flex items-center justify-between">
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          {canUpdate && (
            <Button variant="secondary" onClick={() => { save.reset(); setDraft({ ...empty }); }}>
              {t("new")}
            </Button>
          )}
        </header>
        <p className="text-sm text-slate-600">{t("intro")}</p>
        {calendars.isPending ? (
          <p className="text-slate-500">{t("loading")}</p>
        ) : calendars.isError ? (
          <p className="text-red-700">{t("loadError")}</p>
        ) : calendars.data.length === 0 ? (
          <p className="rounded-md bg-amber-50 p-3 text-sm text-amber-800">{t("empty")}</p>
        ) : (
          <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white text-sm" aria-label={t("title")}>
            {calendars.data.map((c) => (
              <li key={c.id}>
                <button
                  className={`flex w-full items-center justify-between gap-2 px-3 py-2 text-left hover:bg-slate-50 ${c.id === current?.id && !draft ? "bg-slate-50 font-medium" : ""}`}
                  onClick={() => { save.reset(); setSelected(c.id); setDraft(null); }}
                >
                  <span>{c.name}</span>
                  {c.is_default && <span className="rounded bg-emerald-50 px-2 py-0.5 text-xs text-emerald-700">{t("default")}</span>}
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="space-y-6">
        {form && (
          <fieldset className="space-y-3 rounded-md border border-slate-200 bg-white p-4 text-sm" disabled={!canUpdate}>
            <legend className="px-1 font-semibold">{form.id ? t("settings") : t("new")}</legend>
            <label className="block">
              <span className="block text-slate-600">{t("name")}</span>
              <input className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1" value={form.name} onChange={(e) => setDraft({ ...form, name: e.target.value })} />
            </label>
            <label className="block">
              <span className="block text-slate-600">{t("timezone")}</span>
              <input className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1 font-mono" value={form.timezone} onChange={(e) => setDraft({ ...form, timezone: e.target.value })} />
            </label>
            <div>
              <span className="block text-slate-600">{t("workdays")}</span>
              <div className="mt-1 flex flex-wrap gap-3">
                {WEEKDAYS.map((d) => (
                  <label key={d} className="flex items-center gap-1">
                    <input
                      type="checkbox"
                      checked={form.workdays.includes(d)}
                      onChange={(e) => setDraft({ ...form, workdays: e.target.checked ? [...form.workdays, d].sort() : form.workdays.filter((x) => x !== d) })}
                    />
                    {t(`weekday.${d}`)}
                  </label>
                ))}
              </div>
            </div>
            <label className="flex items-center gap-2">
              <input type="checkbox" checked={form.isDefault} disabled={form.wasDefault} onChange={(e) => setDraft({ ...form, isDefault: e.target.checked })} />
              {t("makeDefault")}
            </label>
            {form.wasDefault && <p className="text-xs text-slate-500">{t("defaultHint")}</p>}
            {saveError && <p className="text-red-700" role="alert">{saveError}</p>}
            {canUpdate && (
              <div className="flex gap-2">
                <Button onClick={submit} disabled={save.isPending || form.name.trim() === "" || form.workdays.length === 0}>
                  {t("save")}
                </Button>
                {draft && <Button variant="secondary" onClick={() => { save.reset(); setDraft(null); }}>{t("cancel")}</Button>}
              </div>
            )}
          </fieldset>
        )}
        {current && !draft && <Holidays key={current.id} calendar={current} canUpdate={canUpdate} locale={locale} />}
      </section>
      </div>
    </main>
  );
}

function Holidays({ calendar, canUpdate, locale }: { calendar: BusinessCalendar; canUpdate: boolean; locale: Locale }) {
  const t = useTranslations("calendar");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [year, setYear] = useState(() => new Date().getFullYear());
  const holidays = useHolidays(client, calendar.id, year);
  const m = useHolidayMutations(client, calendar.id);
  const [date, setDate] = useState("");
  const [name, setName] = useState("");
  const [importing, setImporting] = useState(false);
  // ImportWizard calls onDone when its status becomes done; keep the callback stable so it runs once.
  const refresh = useRef(m.refresh);
  useEffect(() => { refresh.current = m.refresh; });
  const onImported = useCallback(() => { void refresh.current(); }, []);

  const shownYear = locale === "th" ? year + 543 : year;
  const add = () => m.put.mutate({ date, name }, { onSuccess: () => { setDate(""); setName(""); } });

  return (
    <div className="space-y-3 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-lg font-semibold">{t("holidaysOf", { name: calendar.name })}</h2>
        <div className="flex items-center gap-2">
          <Button variant="secondary" onClick={() => setYear(year - 1)} aria-label={t("prevYear")}>‹</Button>
          <span className="min-w-16 text-center font-medium" data-testid="holiday-year">{shownYear}</span>
          <Button variant="secondary" onClick={() => setYear(year + 1)} aria-label={t("nextYear")}>›</Button>
        </div>
      </header>

      {holidays.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : holidays.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : holidays.data.length === 0 ? (
        <p className="text-slate-500">{t("noHolidays")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr>
              <th className="px-3 py-2">{t("date")}</th>
              <th className="px-3 py-2">{t("holidayName")}</th>
              {canUpdate && <th className="px-3 py-2" />}
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {holidays.data.map((h) => (
              <tr key={h.date}>
                <td className="px-3 py-2">{formatDate(h.date, locale, { weekday: "short" })}</td>
                <td className="px-3 py-2">{h.name}</td>
                {canUpdate && (
                  <td className="px-3 py-2 text-right">
                    <button className="text-red-700 hover:underline" onClick={() => m.remove.mutate(h.date)} disabled={m.remove.isPending}>
                      {t("remove")}
                    </button>
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {canUpdate && (
        <div className="flex flex-wrap items-end gap-2 rounded-md border border-slate-200 bg-white p-3">
          <label>
            <span className="block text-slate-600">{t("date")}</span>
            <input type="date" className="mt-1 rounded-md border border-slate-300 px-2 py-1" value={date} onChange={(e) => setDate(e.target.value)} />
          </label>
          <label className="flex-1">
            <span className="block text-slate-600">{t("holidayName")}</span>
            <input className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1" value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <Button onClick={add} disabled={!date || !name.trim() || m.put.isPending}>{t("add")}</Button>
          {m.put.isError && <p className="w-full text-red-700" role="alert">{t("saveError")}</p>}
        </div>
      )}

      {canUpdate && (
        <div className="space-y-2">
          <Button variant="secondary" onClick={() => setImporting(!importing)}>{importing ? t("closeImport") : t("import")}</Button>
          {importing && (
            <>
              <p className="text-xs text-slate-500">{t("importHint")}</p>
              <ImportWizard importType={HOLIDAY_IMPORT} onDone={onImported} />
            </>
          )}
        </div>
      )}
    </div>
  );
}

type SettingsDraft = { rowVersion: number; language: "th" | "en"; dateEra: "BE" | "CE"; logoFileId?: string; themeColor: string; accentColor: string };

function fromSettings(s: OrgSettings): SettingsDraft {
  return {
    rowVersion: s.row_version,
    language: (s.default_language as "th" | "en") ?? "th",
    dateEra: (s.date_era as "BE" | "CE") ?? "BE",
    logoFileId: s.branding?.logo_file_id,
    themeColor: s.branding?.theme_color ?? "",
    accentColor: s.branding?.accent_color ?? "",
  };
}

function GeneralSettings({ canUpdate }: { canUpdate: boolean }) {
  const t = useTranslations("calendar.general");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const settings = useOrgSettings(client);
  const save = useSaveOrgSettings(client);
  const [draft, setDraft] = useState<SettingsDraft | null>(null);
  const [saved, setSaved] = useState(false);

  const form = draft ?? (settings.data ? fromSettings(settings.data) : null);
  const set = (p: Partial<SettingsDraft>) => { setSaved(false); if (form) setDraft({ ...form, ...p }); };

  if (settings.isPending) return <p className="text-slate-500">{t("loading")}</p>;
  if (settings.isError || !form) return <p className="text-red-700">{t("loadError")}</p>;

  const submit = () => {
    save.mutate(
      {
        rowVersion: form.rowVersion,
        input: {
          default_language: form.language,
          date_era: form.dateEra,
          branding: { logo_file_id: form.logoFileId, theme_color: form.themeColor || undefined, accent_color: form.accentColor || undefined },
        },
      },
      { onSuccess: () => { setDraft(null); setSaved(true); } },
    );
  };
  const saveError = save.error ? (problemCode(save.error) === "org.invalid_settings" ? t("invalid") : t("saveError", { detail: problemCode(save.error) ?? "" })) : null;

  return (
    <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 text-sm sm:grid-cols-2" disabled={!canUpdate}>
      <legend className="px-1 font-semibold">{t("title")}</legend>
      <p className="text-slate-600 sm:col-span-2">{t("intro")}</p>
      <label>
        <span className="block text-slate-600">{t("language")}</span>
        <select className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1" value={form.language} onChange={(e) => set({ language: e.target.value as "th" | "en" })}>
          <option value="th">{t("th")}</option>
          <option value="en">{t("en")}</option>
        </select>
      </label>
      <label>
        <span className="block text-slate-600">{t("dateEra")}</span>
        <select className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1" value={form.dateEra} onChange={(e) => set({ dateEra: e.target.value as "BE" | "CE" })}>
          <option value="BE">{t("BE")}</option>
          <option value="CE">{t("CE")}</option>
        </select>
      </label>
      <label>
        <span className="block text-slate-600">{t("themeColor")}</span>
        <input type="color" className="mt-1 h-9 w-full rounded-md border border-slate-300" value={form.themeColor || "#0b5fff"} onChange={(e) => set({ themeColor: e.target.value })} />
      </label>
      <label>
        <span className="block text-slate-600">{t("accentColor")}</span>
        <input type="color" className="mt-1 h-9 w-full rounded-md border border-slate-300" value={form.accentColor || "#0b5fff"} onChange={(e) => set({ accentColor: e.target.value })} />
      </label>
      <div className="sm:col-span-2">
        <span className="block text-slate-600">{t("logo")}{form.logoFileId && <span className="ml-2 text-emerald-700">· {t("logoSet")}</span>}</span>
        {canUpdate && <FileUploader accept=".png,.jpg,.jpeg" hint={t("logoLimits")} onUploaded={(f) => set({ logoFileId: f.id })} />}
      </div>
      {saveError && <p className="text-red-700 sm:col-span-2" role="alert">{saveError}</p>}
      {saved && <p className="text-emerald-800 sm:col-span-2" role="status">{t("saved")}</p>}
      {canUpdate && <Button onClick={submit} disabled={save.isPending}>{t("save")}</Button>}
    </fieldset>
  );
}
