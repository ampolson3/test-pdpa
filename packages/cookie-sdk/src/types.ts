// Wire shapes this SDK reads from /public/v1/cookie-banners/{key} — a small hand-written mirror of
// api/openapi/openapi.yaml's CookiePublicBanner/CookieCategory/CookieBannerTexts, not the generated
// @pdpa/api-client (that client pulls in its own fetch wrapper and the whole generated schema — far more
// than this standalone, CDN-embedded bundle can afford against its own 30 KB gzip ceiling, CON-01's own
// acceptance criterion).

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

export interface PublicBanner {
  /** The published banner_config's own id — pass back as banner_config_id when submitting consent */
  id: string;
  domain_id: string;
  version_no: number;
  layout: 'bar' | 'modal' | 'corner';
  texts: BannerTexts;
  categories: Category[];
}

export type ConsentAction = 'accept_all' | 'reject_all' | 'custom';

export interface StoredConsent {
  version_no: number;
  action: ConsentAction;
  choices: Record<string, boolean>;
  decided_at: string;
}
