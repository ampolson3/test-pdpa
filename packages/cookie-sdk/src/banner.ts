import type { BannerTexts, BannerTheme, Category, PublicBanner } from './types';

export interface BannerCallbacks {
  onAcceptAll: () => void;
  onRejectAll: () => void;
  onSaveCustom: (choices: Record<string, boolean>) => void;
}

function documentLang(): string {
  return typeof document !== 'undefined' ? document.documentElement.lang ?? '' : '';
}

function lang(documentLangOverride: string): 'th' | 'en' {
  return documentLangOverride.toLowerCase().startsWith('en') ? 'en' : 'th';
}

function text(th: string, en: string | undefined, documentLangOverride: string): string {
  return lang(documentLangOverride) === 'en' && en ? en : th;
}

/** CON-06: browser languages beyond th/en — matched against a tenant's own banner translations, falling
 * back to th/en when the browser's language (or none of its preferences) has no translation on file. */
function browserLanguages(): string[] {
  const nav = typeof navigator !== 'undefined' ? navigator : undefined;
  const list = (nav?.languages && nav.languages.length > 0 ? [...nav.languages] : [nav?.language ?? '']).filter(Boolean);
  return list.map((l) => l.toLowerCase().split('-')[0]);
}

interface ResolvedCopy {
  title: string;
  description: string;
  accept_all: string;
  reject_all: string;
  settings: string;
}

/** CON-06's own acceptance criterion: the SDK picks a tenant-added extra language when the visitor's own
 * browser prefers one that has a translation on file; otherwise it falls back to the existing th/en logic.
 * `languages`/`documentLangOverride` are overridable for unit tests that run with no DOM/navigator; real
 * calls (renderBanner) omit them and this reads the actual browser state. */
export function resolveCopy(texts: BannerTexts, languages?: string[], documentLangOverride?: string): ResolvedCopy {
  const translations = texts.translations ?? {};
  for (const code of languages ?? browserLanguages()) {
    if (code === 'th' || code === 'en') break; // th/en already have their own dedicated fields/logic
    const tr = translations[code.toLowerCase().split('-')[0]];
    if (tr) {
      return { title: tr.title, description: tr.description ?? '', accept_all: tr.accept_all, reject_all: tr.reject_all, settings: tr.settings };
    }
  }
  const dl = documentLangOverride ?? documentLang();
  return {
    title: text(texts.title_th, texts.title_en, dl),
    description: text(texts.description_th ?? '', texts.description_en, dl),
    accept_all: text(texts.accept_all_th, texts.accept_all_en, dl),
    reject_all: text(texts.reject_all_th, texts.reject_all_en, dl),
    settings: text(texts.settings_th, texts.settings_en, dl),
  };
}

const STYLE = `
.pdpa-cc{position:fixed;left:0;right:0;bottom:0;z-index:2147483000;background:var(--pdpa-cc-bg,#fff);color:var(--pdpa-cc-text,#1a1a1a);
box-shadow:0 -2px 12px rgba(0,0,0,.15);font:14px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Sarabun,sans-serif;
padding:16px;box-sizing:border-box}
.pdpa-cc[data-position="top"]{top:0;bottom:auto;box-shadow:0 2px 12px rgba(0,0,0,.15)}
.pdpa-cc[data-layout="modal"]{position:fixed;inset:0;display:flex;align-items:center;justify-content:center;
background:rgba(0,0,0,.5)}
.pdpa-cc[data-layout="modal"] .pdpa-cc-box{background:var(--pdpa-cc-bg,#fff);border-radius:8px;max-width:480px;padding:24px}
.pdpa-cc[data-layout="corner"]{left:auto;right:16px;bottom:16px;max-width:360px;border-radius:8px}
.pdpa-cc[data-layout="corner"][data-position="top"]{top:16px;bottom:auto}
.pdpa-cc[data-layout="corner"][data-position="center"]{top:50%;bottom:auto;transform:translateY(-50%)}
.pdpa-cc-logo{height:24px;margin-bottom:8px;display:block}
.pdpa-cc-title{font-weight:600;font-size:16px;margin:0 0 6px}
.pdpa-cc-desc{margin:0 0 12px;color:var(--pdpa-cc-text,#444)}
.pdpa-cc-actions{display:flex;gap:8px;flex-wrap:wrap}
.pdpa-cc-btn{flex:1 1 auto;min-width:120px;padding:10px 16px;border-radius:6px;border:1px solid var(--pdpa-cc-primary,#1a1a1a);
background:var(--pdpa-cc-primary,#1a1a1a);color:var(--pdpa-cc-btn-text,#fff);font-size:14px;font-weight:600;cursor:pointer;text-align:center}
.pdpa-cc-btn.pdpa-cc-secondary{background:var(--pdpa-cc-bg,#fff);color:var(--pdpa-cc-primary,#1a1a1a)}
.pdpa-cc-cats{margin:12px 0}
.pdpa-cc-cat{display:flex;align-items:flex-start;gap:8px;padding:8px 0;border-top:1px solid #eee}
.pdpa-cc-cat label{font-weight:600;display:block}
.pdpa-cc-cat p{margin:2px 0 0;color:#666;font-size:13px}
`;

function injectStyle(): void {
  if (document.getElementById('pdpa-cc-style')) return;
  const style = document.createElement('style');
  style.id = 'pdpa-cc-style';
  style.textContent = STYLE;
  document.head.appendChild(style);
}

// CON-06: a tenant's own limited custom CSS, already length-capped and screened for script/navigation
// vectors server-side (cookie.validateTheme) — injected as its own <style> tag, replaced on every banner
// fetch so an edited/removed theme takes effect without a stale rule lingering from a previous publish.
function injectCustomCSS(css: string | undefined): void {
  const existing = document.getElementById('pdpa-cc-custom-style');
  if (existing) existing.remove();
  if (!css) return;
  const style = document.createElement('style');
  style.id = 'pdpa-cc-custom-style';
  style.textContent = css;
  document.head.appendChild(style);
}

function applyTheme(root: HTMLElement, theme: BannerTheme | undefined): void {
  if (!theme) return;
  if (theme.primary_color) root.style.setProperty('--pdpa-cc-primary', theme.primary_color);
  if (theme.text_color) root.style.setProperty('--pdpa-cc-text', theme.text_color);
  if (theme.button_text_color) root.style.setProperty('--pdpa-cc-btn-text', theme.button_text_color);
  if (theme.position) root.setAttribute('data-position', theme.position);
  injectCustomCSS(theme.custom_css);
}

// renderBanner builds the acceptance criterion's own three equal-weight controls — accept-all and
// reject-all are both one click away, same size and visual weight (CON-01's own "ปฏิเสธทั้งหมดได้ในคลิก
// เดียว...น้ำหนักเท่ากัน") — plus a settings panel of per-category toggles, none pre-ticked unless the
// category is required (is_required, e.g. "necessary").
export function renderBanner(banner: PublicBanner, cb: BannerCallbacks): HTMLElement {
  injectStyle();
  const copy = resolveCopy(banner.texts);
  const root = document.createElement('div');
  root.className = 'pdpa-cc';
  root.setAttribute('data-layout', banner.layout);
  root.setAttribute('role', 'dialog');
  root.setAttribute('aria-label', copy.title);
  applyTheme(root, banner.theme);

  const box = document.createElement('div');
  box.className = 'pdpa-cc-box';

  if (banner.theme?.logo_url) {
    const logo = document.createElement('img');
    logo.className = 'pdpa-cc-logo';
    logo.src = banner.theme.logo_url;
    logo.alt = '';
    box.appendChild(logo);
  }

  const title = document.createElement('p');
  title.className = 'pdpa-cc-title';
  title.textContent = copy.title;
  box.appendChild(title);

  if (copy.description) {
    const p = document.createElement('p');
    p.className = 'pdpa-cc-desc';
    p.textContent = copy.description;
    box.appendChild(p);
  }

  const catsEl = document.createElement('div');
  catsEl.className = 'pdpa-cc-cats';
  catsEl.style.display = 'none';
  const checkboxes = new Map<string, HTMLInputElement>();
  const sorted = [...banner.categories].sort((a, b) => a.display_order - b.display_order);
  for (const c of sorted) {
    catsEl.appendChild(renderCategoryRow(c, checkboxes));
  }
  box.appendChild(catsEl);

  const actions = document.createElement('div');
  actions.className = 'pdpa-cc-actions';

  const acceptBtn = makeButton(copy.accept_all, false);
  acceptBtn.addEventListener('click', () => cb.onAcceptAll());

  const rejectBtn = makeButton(copy.reject_all, false);
  rejectBtn.addEventListener('click', () => cb.onRejectAll());

  const settingsBtn = makeButton(copy.settings, true);
  let settingsOpen = false;
  settingsBtn.addEventListener('click', () => {
    settingsOpen = !settingsOpen;
    catsEl.style.display = settingsOpen ? '' : 'none';
    if (settingsOpen) {
      const saveBtn = makeButton(text('บันทึกการตั้งค่า', 'Save settings', documentLang()), true);
      saveBtn.addEventListener('click', () => {
        const choices: Record<string, boolean> = {};
        for (const [code, el] of checkboxes) choices[code] = el.checked;
        cb.onSaveCustom(choices);
      });
      actions.appendChild(saveBtn);
    }
  });

  actions.appendChild(acceptBtn);
  actions.appendChild(rejectBtn);
  actions.appendChild(settingsBtn);
  box.appendChild(actions);
  root.appendChild(box);
  return root;
}

function renderCategoryRow(c: Category, checkboxes: Map<string, HTMLInputElement>): HTMLElement {
  const row = document.createElement('div');
  row.className = 'pdpa-cc-cat';
  const input = document.createElement('input');
  input.type = 'checkbox';
  input.id = 'pdpa-cc-cat-' + c.code;
  // The acceptance criterion's own "ไม่มีช่องติ๊กล่วงหน้า" — nothing starts checked except a category the
  // tenant itself marked required (e.g. "necessary"), which the visitor cannot turn off anyway.
  input.checked = c.is_required;
  input.disabled = c.is_required;
  checkboxes.set(c.code, input);
  const labelWrap = document.createElement('div');
  const label = document.createElement('label');
  label.htmlFor = input.id;
  label.textContent = text(c.name_th, c.name_en, documentLang());
  const descText = text(c.description_th ?? '', c.description_en, documentLang());
  labelWrap.appendChild(label);
  if (descText) {
    const p = document.createElement('p');
    p.textContent = descText;
    labelWrap.appendChild(p);
  }
  row.appendChild(input);
  row.appendChild(labelWrap);
  return row;
}

function makeButton(labelText: string, secondary: boolean): HTMLButtonElement {
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'pdpa-cc-btn' + (secondary ? ' pdpa-cc-secondary' : '');
  btn.textContent = labelText;
  return btn;
}
