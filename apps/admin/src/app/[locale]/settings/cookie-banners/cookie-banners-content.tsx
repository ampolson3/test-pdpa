"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import {
  createApiClient,
  useLegalEntities,
  useCookieDomains,
  useRegisterCookieDomain,
  useVerifyCookieDomain,
  useCookieCategories,
  useCookieCategoryMutations,
  useCookieBannerConfigs,
  useCookieBannerConfigMutations,
  type CookieDomain,
  type CookieBannerConfig,
  type CookieBannerTexts,
} from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const BTN = "rounded-md bg-blue-700 px-3 py-1.5 text-sm text-white disabled:opacity-60";
const BTN_SECONDARY = "rounded-md border border-slate-300 px-2 py-1 text-xs";

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

const EMPTY_TEXTS: CookieBannerTexts = {
  title_th: "", title_en: "", description_th: "", description_en: "",
  accept_all_th: "", accept_all_en: "", reject_all_th: "", reject_all_en: "", settings_th: "", settings_en: "",
};

export function CookieBannersContent({ apiBase }: { apiBase: string }) {
  const t = useTranslations("cookieBanners");
  const canRead = usePermission("cookie.banner.read");
  const canCreate = usePermission("cookie.banner.create");
  const canUpdate = usePermission("cookie.banner.update");
  const canPublish = usePermission("cookie.banner.publish");
  const canDelete = usePermission("cookie.banner.delete");
  const client = useMemo(() => createApiClient("/api/bff"), []);

  const legalEntities = useLegalEntities(client);
  const domains = useCookieDomains(client);
  const registerDomain = useRegisterCookieDomain(client);
  const verifyDomain = useVerifyCookieDomain(client);
  const categories = useCookieCategories(client);
  const categoryMutations = useCookieCategoryMutations(client);

  const [domain, setDomain] = useState("");
  const [legalEntityId, setLegalEntityId] = useState("");
  const [error, setError] = useState("");

  const [catCode, setCatCode] = useState("");
  const [catNameTh, setCatNameTh] = useState("");
  const [catRequired, setCatRequired] = useState(false);

  const [selectedDomainId, setSelectedDomainId] = useState("");

  if (!canRead) return <main className="mx-auto max-w-4xl p-8 text-slate-600">{t("forbidden")}</main>;

  const entities = legalEntities.data ?? [];
  const domainRows = domains.data ?? [];
  const cats = categories.data ?? [];
  const selectedDomain = domainRows.find((d) => d.id === selectedDomainId);

  const create = () => {
    if (!domain.trim() || !legalEntityId) return;
    setError("");
    registerDomain.mutate({ domain: domain.trim(), legalEntityId }, {
      onSuccess: () => { setDomain(""); setLegalEntityId(""); },
      onError: (e) => setError(detail(e)),
    });
  };

  const createCategory = () => {
    if (!catCode.trim() || !catNameTh.trim()) return;
    categoryMutations.create.mutate({ code: catCode.trim(), nameTh: catNameTh.trim(), isRequired: catRequired, displayOrder: cats.length }, {
      onSuccess: () => { setCatCode(""); setCatNameTh(""); setCatRequired(false); },
    });
  };

  return (
    <main className="mx-auto max-w-4xl space-y-8 p-4 sm:p-8">
      <div>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-sm text-slate-600">{t("intro")}</p>
      </div>

      <section className="space-y-2 rounded-lg border border-slate-200 bg-white p-4">
        <h2 className="font-medium">{t("domainsTitle")}</h2>
        {canCreate && (
          <div className="flex flex-wrap items-end gap-2">
            <label className="text-sm">
              {t("domain")}
              <input value={domain} onChange={(e) => setDomain(e.target.value)} className={INPUT} placeholder="example.com" data-testid="domain-input" />
            </label>
            <label className="text-sm">
              {t("legalEntity")}
              <select value={legalEntityId} onChange={(e) => setLegalEntityId(e.target.value)} className={INPUT} data-testid="domain-legal-entity">
                <option value="">{t("legalEntityPlaceholder")}</option>
                {entities.map((e) => (
                  <option key={e.id} value={e.id}>{e.name_th}</option>
                ))}
              </select>
            </label>
            <button type="button" onClick={create} disabled={registerDomain.isPending} className={BTN} data-testid="register-domain">
              {t("registerDomain")}
            </button>
          </div>
        )}
        {error && <p className="text-sm text-rose-700" role="alert">{error}</p>}

        <table className="w-full text-sm">
          <thead className="text-left text-slate-500">
            <tr>
              <th className="py-1">{t("domain")}</th>
              <th>{t("status")}</th>
              <th>{t("embedKey")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {domainRows.map((d) => (
              <tr key={d.id} className="border-t border-slate-100" data-testid={`domain-row-${d.id}`}>
                <td className="py-1.5">{d.domain}</td>
                <td>{t(`statuses.${d.status}`)}</td>
                <td className="break-all font-mono text-xs">{d.public_key ?? t("noKeyYet")}</td>
                <td className="space-x-2 text-right">
                  {d.status !== "verified" && canUpdate && (
                    <button type="button" onClick={() => verifyDomain.mutate({ domain: d })} className={BTN_SECONDARY} data-testid={`verify-${d.id}`}>
                      {t("verify")}
                    </button>
                  )}
                  <button type="button" onClick={() => setSelectedDomainId(d.id)} className={BTN_SECONDARY} data-testid={`manage-${d.id}`}>
                    {t("manageBanner")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {domainRows.length === 0 && <p className="text-sm text-slate-500">{t("noDomains")}</p>}
      </section>

      <section className="space-y-2 rounded-lg border border-slate-200 bg-white p-4">
        <h2 className="font-medium">{t("categoriesTitle")}</h2>
        {canCreate && (
          <div className="flex flex-wrap items-end gap-2">
            <label className="text-sm">
              {t("categoryCode")}
              <input value={catCode} onChange={(e) => setCatCode(e.target.value)} className={INPUT} placeholder="analytics" data-testid="category-code" />
            </label>
            <label className="text-sm">
              {t("categoryNameTh")}
              <input value={catNameTh} onChange={(e) => setCatNameTh(e.target.value)} className={INPUT} data-testid="category-name" />
            </label>
            <label className="flex items-center gap-1 text-sm">
              <input type="checkbox" checked={catRequired} onChange={(e) => setCatRequired(e.target.checked)} data-testid="category-required" />
              {t("categoryRequired")}
            </label>
            <button type="button" onClick={createCategory} disabled={categoryMutations.create.isPending} className={BTN} data-testid="create-category">
              {t("addCategory")}
            </button>
          </div>
        )}
        <ul className="divide-y divide-slate-100 text-sm">
          {cats.map((c) => (
            <li key={c.id} className="flex items-center justify-between py-1.5" data-testid={`category-row-${c.id}`}>
              <span>
                {c.name_th} <span className="text-slate-400">({c.code})</span>
                {c.is_required && <span className="ml-2 rounded bg-slate-100 px-1.5 py-0.5 text-xs">{t("categoryRequired")}</span>}
              </span>
              {canDelete && (
                <button type="button" onClick={() => categoryMutations.remove.mutate(c.id)} className="text-xs text-rose-700">
                  {t("removeCategory")}
                </button>
              )}
            </li>
          ))}
        </ul>
        {cats.length === 0 && <p className="text-sm text-slate-500">{t("noCategories")}</p>}
      </section>

      {selectedDomain && (
        <BannerSection
          client={client}
          domain={selectedDomain}
          apiBase={apiBase}
          canCreate={canCreate}
          canUpdate={canUpdate}
          canPublish={canPublish}
          onClose={() => setSelectedDomainId("")}
        />
      )}
    </main>
  );
}

function BannerSection({ client, domain, apiBase, canCreate, canUpdate, canPublish, onClose }: {
  client: ReturnType<typeof createApiClient>;
  domain: CookieDomain;
  apiBase: string;
  canCreate: boolean;
  canUpdate: boolean;
  canPublish: boolean;
  onClose: () => void;
}) {
  const t = useTranslations("cookieBanners");
  const configs = useCookieBannerConfigs(client, domain.id);
  const m = useCookieBannerConfigMutations(client, domain.id);
  const [editing, setEditing] = useState<CookieBannerConfig | null>(null);
  const [creating, setCreating] = useState(false);
  const [layout, setLayout] = useState<"bar" | "modal" | "corner">("bar");
  const [texts, setTexts] = useState<CookieBannerTexts>(EMPTY_TEXTS);

  const rows = configs.data ?? [];
  const draft = rows.find((c) => c.status === "draft");
  const showForm = editing !== null || creating;

  const startEdit = (c: CookieBannerConfig) => {
    setEditing(c);
    setCreating(false);
    setLayout(c.layout);
    setTexts(c.texts);
  };

  const startNew = () => {
    setEditing(null);
    setCreating(true);
    setLayout("bar");
    setTexts(EMPTY_TEXTS);
  };

  const save = () => {
    const input = { layout, texts };
    if (editing) {
      m.update.mutate({ config: editing, input }, { onSuccess: (c) => setEditing(c) });
    } else {
      m.create.mutate(input, { onSuccess: (c) => { setEditing(c); setCreating(false); } });
    }
  };

  const snippet = domain.public_key
    ? `<script src="${apiBase}/static/pdpa-cookie-sdk.js" data-domain-key="${domain.public_key}" data-api-base="${apiBase}"></script>`
    : "";

  return (
    <section className="space-y-3 rounded-lg border border-slate-200 bg-white p-4">
      <div className="flex items-center justify-between">
        <h2 className="font-medium">{t("bannerFor", { domain: domain.domain })}</h2>
        <button type="button" onClick={onClose} className="text-xs text-slate-500">{t("close")}</button>
      </div>

      {snippet && (
        <div>
          <p className="text-sm font-medium">{t("installSnippet")}</p>
          <pre className="mt-1 overflow-x-auto rounded bg-slate-900 p-2 text-xs text-slate-100" data-testid="install-snippet">{snippet}</pre>
        </div>
      )}

      <table className="w-full text-sm">
        <thead className="text-left text-slate-500">
          <tr><th className="py-1">{t("version")}</th><th>{t("status")}</th><th /></tr>
        </thead>
        <tbody>
          {rows.map((c) => (
            <tr key={c.id} className="border-t border-slate-100" data-testid={`banner-row-${c.id}`}>
              <td className="py-1.5">v{c.version_no}</td>
              <td>{t(`bannerStatuses.${c.status}`)}</td>
              <td className="space-x-2 text-right">
                {c.status === "draft" && canUpdate && (
                  <button type="button" onClick={() => startEdit(c)} className={BTN_SECONDARY}>{t("edit")}</button>
                )}
                {c.status === "draft" && canPublish && (
                  <button type="button" onClick={() => m.publish.mutate(c)} className={BTN_SECONDARY} data-testid={`publish-${c.id}`}>{t("publish")}</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {rows.length === 0 && <p className="text-sm text-slate-500">{t("noBannerVersions")}</p>}

      {canCreate && !draft && !showForm && (
        <button type="button" onClick={startNew} className={BTN_SECONDARY} data-testid="new-banner-draft">{t("newDraft")}</button>
      )}

      {showForm && (
        <BannerForm layout={layout} setLayout={setLayout} texts={texts} setTexts={setTexts} onSave={save} saving={m.create.isPending || m.update.isPending} />
      )}
    </section>
  );
}

function BannerForm({ layout, setLayout, texts, setTexts, onSave, saving }: {
  layout: "bar" | "modal" | "corner";
  setLayout: (v: "bar" | "modal" | "corner") => void;
  texts: CookieBannerTexts;
  setTexts: (t: CookieBannerTexts) => void;
  onSave: () => void;
  saving: boolean;
}) {
  const t = useTranslations("cookieBanners");
  const field = (key: keyof CookieBannerTexts, label: string) => (
    <label className="block text-sm">
      {label}
      <input value={texts[key] ?? ""} onChange={(e) => setTexts({ ...texts, [key]: e.target.value })} className={INPUT} data-testid={`banner-${key}`} />
    </label>
  );
  return (
    <div className="space-y-2 border-t border-slate-100 pt-3">
      <label className="block text-sm">
        {t("layout")}
        <select value={layout} onChange={(e) => setLayout(e.target.value as typeof layout)} className={INPUT}>
          <option value="bar">{t("layouts.bar")}</option>
          <option value="modal">{t("layouts.modal")}</option>
          <option value="corner">{t("layouts.corner")}</option>
        </select>
      </label>
      {field("title_th", t("fields.titleTh"))}
      {field("description_th", t("fields.descriptionTh"))}
      {field("accept_all_th", t("fields.acceptAllTh"))}
      {field("reject_all_th", t("fields.rejectAllTh"))}
      {field("settings_th", t("fields.settingsTh"))}
      <button type="button" onClick={onSave} disabled={saving} className={BTN} data-testid="save-banner-draft">{t("save")}</button>
    </div>
  );
}
