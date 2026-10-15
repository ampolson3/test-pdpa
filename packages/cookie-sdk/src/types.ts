// Wire shapes this SDK reads from /public/v1/cookie-banners/{key} — a small hand-written mirror of
// api/openapi/openapi.yaml's CookiePublicBanner/CookieCategory/CookieBannerTexts, not the generated
// @pdpa/api-client (that client pulls in its own fetch wrapper and the whole generated schema — far more
// than this standalone, CDN-embedded bundle can afford against its own 30 KB gzip ceiling, CON-01's own
// acceptance criterion).

export interface BannerTranslation {
  title: string;
  description?: string;
  accept_all: string;
  reject_all: string;
  settings: string;
}

export interface BannerTexts {
  title_th: string;
  title_en?: string;
  description_th?: string;
  description_en?: string;
  accept_all_th: string;
  accept_all_en?: string;
  reject_all_th: string;
  reject_all_en?: string;
  settings_th: string;
  settings_en?: string;
  /** CON-06: extra languages beyond th/en, keyed by ISO code — picked by the visitor's own browser language */
  translations?: Record<string, BannerTranslation>;
}

/** CON-06: design customization — colors, logo, docking edge and a tenant's own limited custom CSS */
export interface BannerTheme {
  primary_color?: string;
  text_color?: string;
  button_text_color?: string;
  logo_url?: string;
  position?: 'top' | 'bottom' | 'center';
  custom_css?: string;
}

export interface Category {
  id: string;
  code: string;
  name_th: string;
  name_en?: string;
  description_th?: string;
  description_en?: string;
  is_required: boolean;
  display_order: number;
}

/** CON-02: which scripts belong to which category — the SDK enables/disables matching scripts the moment a
 * visitor's preference changes. */
export interface ScriptRule {
  pattern: string;
  category_code: string;
  action: 'block' | 'allow';
}

export interface PublicBanner {
  /** The published banner_config's own id — pass back as banner_config_id when submitting consent */
  id: string;
  domain_id: string;
  version_no: number;
  layout: 'bar' | 'modal' | 'corner';
  theme?: BannerTheme;
  texts: BannerTexts;
  categories: Category[];
  script_rules?: ScriptRule[];
}

export type ConsentAction = 'accept_all' | 'reject_all' | 'custom';

export interface StoredConsent {
  version_no: number;
  action: ConsentAction;
  choices: Record<string, boolean>;
  decided_at: string;
}
