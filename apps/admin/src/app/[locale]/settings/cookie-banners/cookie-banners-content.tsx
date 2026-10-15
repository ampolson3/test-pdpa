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
  useCookieCookies,
  useCookieCookieMutations,
  useCookieScriptRules,
  useCookieScriptRuleMutations,
  useCookieScans,
  useStartCookieScan,
  useCookieScanFindings,
  useClassifyCookieScanFinding,
  type CookieDomain,
  type CookieBannerConfig,
  type CookieBannerTexts,
  type CookieCategory,
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
  const canScanRead = usePermission("cookie.scan.read");
  const canScanCreate = usePermission("cookie.scan.create");
  const canScanUpdate = usePermission("cookie.scan.update");
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
          categories={cats}
          apiBase={apiBase}
          canCreate={canCreate}
          canUpdate={canUpdate}
          canPublish={canPublish}
          canDelete={canDelete}
          canScanRead={canScanRead}
          canScanCreate={canScanCreate}
          canScanUpdate={canScanUpdate}
          onClose={() => setSelectedDomainId("")}
        />
      )}
    </main>
  );
}

function BannerSection({ client, domain, categories, apiBase, canCreate, canUpdate, canPublish, canDelete, canScanRead, canScanCreate, canScanUpdate, onClose }: {
  client: ReturnType<typeof createApiClient>;
  domain: CookieDomain;
  categories: CookieCategory[];
  apiBase: string;
  canCreate: boolean;
  canUpdate: boolean;
  canPublish: boolean;
  canDelete: boolean;
  canScanRead: boolean;
  canScanCreate: boolean;
  canScanUpdate: boolean;
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

      <CookiesSection client={client} domain={domain} categories={categories} canCreate={canCreate} canDelete={canDelete} />
      <ScriptRulesSection client={client} domain={domain} categories={categories} canCreate={canCreate} canDelete={canDelete} />
      {canScanRead && (
        <ScanSection client={client} domain={domain} categories={categories} canScanCreate={canScanCreate} canScanUpdate={canScanUpdate} />
      )}
    </section>
  );
}

function categoryLabel(categories: CookieCategory[], id: string | null | undefined): string {
  return categories.find((c) => c.id === id)?.name_th ?? "";
}

function CookiesSection({ client, domain, categories, canCreate, canDelete }: {
  client: ReturnType<typeof createApiClient>;
  domain: CookieDomain;
  categories: CookieCategory[];
  canCreate: boolean;
  canDelete: boolean;
}) {
  const t = useTranslations("cookieBanners");
  const list = useCookieCookies(client, domain.id);
  const m = useCookieCookieMutations(client, domain.id);
  const [name, setName] = useState("");
  const [providerDomain, setProviderDomain] = useState("");
  const [categoryId, setCategoryId] = useState("");
  const [storageType, setStorageType] = useState<"http_cookie" | "js_cookie" | "local_storage" | "session_storage" | "pixel">("http_cookie");

  const rows = list.data ?? [];

  const create = () => {
    if (!name.trim() || !providerDomain.trim()) return;
    m.create.mutate({ name: name.trim(), provider_domain: providerDomain.trim(), category_id: categoryId || undefined, storage_type: storageType }, {
      onSuccess: () => { setName(""); setProviderDomain(""); setCategoryId(""); },
    });
  };

  return (
    <div className="space-y-2 border-t border-slate-100 pt-3">
      <h3 className="font-medium">{t("cookiesTitle")}</h3>
      {canCreate && (
        <div className="flex flex-wrap items-end gap-2">
          <label className="text-sm">
            {t("cookieName")}
            <input value={name} onChange={(e) => setName(e.target.value)} className={INPUT} placeholder="_ga" data-testid="cookie-name" />
          </label>
          <label className="text-sm">
            {t("cookieProviderDomain")}
            <input value={providerDomain} onChange={(e) => setProviderDomain(e.target.value)} className={INPUT} placeholder="google.com" data-testid="cookie-provider" />
          </label>
          <label className="text-sm">
            {t("cookieCategory")}
            <select value={categoryId} onChange={(e) => setCategoryId(e.target.value)} className={INPUT} data-testid="cookie-category">
              <option value="">{t("cookieNoCategory")}</option>
              {categories.map((c) => (
                <option key={c.id} value={c.id}>{c.name_th}</option>
              ))}
            </select>
          </label>
          <label className="text-sm">
            {t("cookieStorageType")}
            <select value={storageType} onChange={(e) => setStorageType(e.target.value as typeof storageType)} className={INPUT}>
              <option value="http_cookie">http_cookie</option>
              <option value="js_cookie">js_cookie</option>
              <option value="local_storage">local_storage</option>
              <option value="session_storage">session_storage</option>
              <option value="pixel">pixel</option>
            </select>
          </label>
          <button type="button" onClick={create} disabled={m.create.isPending} className={BTN} data-testid="create-cookie">{t("addCookie")}</button>
        </div>
      )}
      <ul className="divide-y divide-slate-100 text-sm">
        {rows.map((r) => (
          <li key={r.id} className="flex items-center justify-between py-1.5" data-testid={`cookie-row-${r.id}`}>
            <span>
              {r.name} <span className="text-slate-400">({r.provider_domain}, {r.storage_type})</span>
              {r.category_id && <span className="ml-2 rounded bg-slate-100 px-1.5 py-0.5 text-xs">{categoryLabel(categories, r.category_id)}</span>}
            </span>
            {canDelete && (
              <button type="button" onClick={() => m.remove.mutate(r.id)} className="text-xs text-rose-700">{t("removeCategory")}</button>
            )}
          </li>
        ))}
      </ul>
      {rows.length === 0 && <p className="text-sm text-slate-500">{t("noCookies")}</p>}
    </div>
  );
}

function ScriptRulesSection({ client, domain, categories, canCreate, canDelete }: {
  client: ReturnType<typeof createApiClient>;
  domain: CookieDomain;
  categories: CookieCategory[];
  canCreate: boolean;
  canDelete: boolean;
}) {
  const t = useTranslations("cookieBanners");
  const list = useCookieScriptRules(client, domain.id);
  const m = useCookieScriptRuleMutations(client, domain.id);
  const [pattern, setPattern] = useState("");
  const [categoryId, setCategoryId] = useState("");
  const [action, setAction] = useState<"block" | "allow">("block");

  const rows = list.data ?? [];

  const create = () => {
    if (!pattern.trim() || !categoryId) return;
    m.create.mutate({ pattern: pattern.trim(), category_id: categoryId, action }, {
      onSuccess: () => setPattern(""),
    });
  };

  return (
    <div className="space-y-2 border-t border-slate-100 pt-3">
      <h3 className="font-medium">{t("scriptRulesTitle")}</h3>
      <p className="text-xs text-slate-500">{t("scriptRulesHint")}</p>
      {canCreate && (
        <div className="flex flex-wrap items-end gap-2">
          <label className="text-sm">
            {t("scriptRulePattern")}
            <input value={pattern} onChange={(e) => setPattern(e.target.value)} className={INPUT} placeholder="googletagmanager.com" data-testid="script-rule-pattern" />
          </label>
          <label className="text-sm">
            {t("cookieCategory")}
            <select value={categoryId} onChange={(e) => setCategoryId(e.target.value)} className={INPUT} data-testid="script-rule-category">
              <option value="">{t("legalEntityPlaceholder")}</option>
              {categories.map((c) => (
                <option key={c.id} value={c.id}>{c.name_th}</option>
              ))}
            </select>
          </label>
          <label className="text-sm">
            {t("scriptRuleAction")}
            <select value={action} onChange={(e) => setAction(e.target.value as typeof action)} className={INPUT}>
              <option value="block">block</option>
              <option value="allow">allow</option>
            </select>
          </label>
          <button type="button" onClick={create} disabled={m.create.isPending} className={BTN} data-testid="create-script-rule">{t("addScriptRule")}</button>
        </div>
      )}
      <ul className="divide-y divide-slate-100 text-sm">
        {rows.map((r) => (
          <li key={r.id} className="flex items-center justify-between py-1.5" data-testid={`script-rule-row-${r.id}`}>
            <span>
              <code>{r.pattern}</code> <span className="text-slate-400">({r.action})</span>
              <span className="ml-2 rounded bg-slate-100 px-1.5 py-0.5 text-xs">{categoryLabel(categories, r.category_id)}</span>
            </span>
            {canDelete && (
              <button type="button" onClick={() => m.remove.mutate(r.id)} className="text-xs text-rose-700">{t("removeCategory")}</button>
            )}
          </li>
        ))}
      </ul>
      {rows.length === 0 && <p className="text-sm text-slate-500">{t("noScriptRules")}</p>}
    </div>
  );
}

function ScanSection({ client, domain, categories, canScanCreate, canScanUpdate }: {
  client: ReturnType<typeof createApiClient>;
  domain: CookieDomain;
  categories: CookieCategory[];
  canScanCreate: boolean;
  canScanUpdate: boolean;
}) {
  const t = useTranslations("cookieBanners");
  const scans = useCookieScans(client, domain.id);
  const startScan = useStartCookieScan(client, domain.id);
  const [openScanId, setOpenScanId] = useState("");

  const rows = scans.data ?? [];
  const running = rows.some((s) => s.status === "queued" || s.status === "running");

  return (
    <div className="space-y-2 border-t border-slate-100 pt-3">
      <div className="flex items-center justify-between">
        <h3 className="font-medium">{t("scansTitle")}</h3>
        {canScanCreate && (
          <button type="button" onClick={() => startScan.mutate()} disabled={startScan.isPending || running} className={BTN} data-testid="start-scan">
            {t("startScan")}
          </button>
        )}
      </div>
      <table className="w-full text-sm">
        <thead className="text-left text-slate-500">
          <tr>
            <th className="py-1">{t("scanStartedAt")}</th>
            <th>{t("status")}</th>
            <th>{t("scanPages")}</th>
            <th>{t("scanNewCookies")}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {rows.map((s) => (
            <tr key={s.id} className="border-t border-slate-100" data-testid={`scan-row-${s.id}`}>
              <td className="py-1.5">{s.started_at ? new Date(s.started_at).toLocaleString() : "—"}</td>
              <td>{t(`scanStatuses.${s.status}`)}</td>
              <td>{s.pages_scanned ?? "—"}</td>
              <td>{s.new_cookies ?? "—"}</td>
              <td className="text-right">
                {s.status === "done" && (
                  <button type="button" onClick={() => setOpenScanId(openScanId === s.id ? "" : s.id)} className={BTN_SECONDARY} data-testid={`view-findings-${s.id}`}>
                    {openScanId === s.id ? t("hideFindings") : t("viewFindings")}
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {rows.length === 0 && <p className="text-sm text-slate-500">{t("noScans")}</p>}
      {openScanId && (
        <ScanFindingsPanel client={client} scanId={openScanId} categories={categories} canScanUpdate={canScanUpdate} />
      )}
    </div>
  );
}

function ScanFindingsPanel({ client, scanId, categories, canScanUpdate }: {
  client: ReturnType<typeof createApiClient>;
  scanId: string;
  categories: CookieCategory[];
  canScanUpdate: boolean;
}) {
  const t = useTranslations("cookieBanners");
  const findings = useCookieScanFindings(client, scanId);
  const classify = useClassifyCookieScanFinding(client, scanId);
  const rows = findings.data ?? [];

  return (
    <div className="space-y-2 rounded-md border border-slate-100 bg-slate-50 p-2">
      <table className="w-full text-sm">
        <thead className="text-left text-slate-500">
          <tr>
            <th className="py-1">{t("findingCookieName")}</th>
            <th>{t("findingCookieDomain")}</th>
            <th>{t("findingSuggestedCategory")}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {rows.map((f) => (
            <tr key={f.id} className="border-t border-slate-200" data-testid={`finding-row-${f.id}`}>
              <td className="py-1.5"><code>{f.cookie_name}</code></td>
              <td>{f.cookie_domain}</td>
              <td>{f.suggested_category ?? "—"}</td>
              <td className="text-right">
                {canScanUpdate && (
                  <select
                    className="rounded-md border border-slate-300 bg-white px-1.5 py-0.5 text-xs"
                    defaultValue=""
                    onChange={(e) => {
                      if (e.target.value) classify.mutate({ findingId: f.id, categoryId: e.target.value });
                    }}
                    data-testid={`classify-${f.id}`}
                  >
                    <option value="">{t("classify")}</option>
                    {categories.map((c) => (
                      <option key={c.id} value={c.id}>{c.name_th}</option>
                    ))}
                  </select>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {rows.length === 0 && <p className="text-sm text-slate-500">{t("noFindings")}</p>}
    </div>
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
