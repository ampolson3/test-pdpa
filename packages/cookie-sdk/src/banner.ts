import type { Category, PublicBanner } from './types';

export interface BannerCallbacks {
  onAcceptAll: () => void;
  onRejectAll: () => void;
  onSaveCustom: (choices: Record<string, boolean>) => void;
}

function lang(): 'th' | 'en' {
  return document.documentElement.lang?.toLowerCase().startsWith('en') ? 'en' : 'th';
}

function text(th: string, en: string | undefined): string {
  return lang() === 'en' && en ? en : th;
}

const STYLE = `
.pdpa-cc{position:fixed;left:0;right:0;bottom:0;z-index:2147483000;background:#fff;color:#1a1a1a;
box-shadow:0 -2px 12px rgba(0,0,0,.15);font:14px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Sarabun,sans-serif;
padding:16px;box-sizing:border-box}
.pdpa-cc[data-layout="modal"]{position:fixed;inset:0;display:flex;align-items:center;justify-content:center;
background:rgba(0,0,0,.5)}
.pdpa-cc[data-layout="modal"] .pdpa-cc-box{background:#fff;border-radius:8px;max-width:480px;padding:24px}
.pdpa-cc[data-layout="corner"]{left:auto;right:16px;bottom:16px;max-width:360px;border-radius:8px}
.pdpa-cc-title{font-weight:600;font-size:16px;margin:0 0 6px}
.pdpa-cc-desc{margin:0 0 12px;color:#444}
.pdpa-cc-actions{display:flex;gap:8px;flex-wrap:wrap}
.pdpa-cc-btn{flex:1 1 auto;min-width:120px;padding:10px 16px;border-radius:6px;border:1px solid #1a1a1a;
background:#1a1a1a;color:#fff;font-size:14px;font-weight:600;cursor:pointer;text-align:center}
.pdpa-cc-btn.pdpa-cc-secondary{background:#fff;color:#1a1a1a}
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

// renderBanner builds the acceptance criterion's own three equal-weight controls — accept-all and
// reject-all are both one click away, same size and visual weight (CON-01's own "ปฏิเสธทั้งหมดได้ในคลิก
// เดียว...น้ำหนักเท่ากัน") — plus a settings panel of per-category toggles, none pre-ticked unless the
// category is required (is_required, e.g. "necessary").
export function renderBanner(banner: PublicBanner, cb: BannerCallbacks): HTMLElement {
  injectStyle();
  const root = document.createElement('div');
  root.className = 'pdpa-cc';
  root.setAttribute('data-layout', banner.layout);
  root.setAttribute('role', 'dialog');
  root.setAttribute('aria-label', text(banner.texts.title_th, banner.texts.title_en));

  const box = document.createElement('div');
  box.className = 'pdpa-cc-box';

  const title = document.createElement('p');
  title.className = 'pdpa-cc-title';
  title.textContent = text(banner.texts.title_th, banner.texts.title_en);
  box.appendChild(title);

  const desc = text(banner.texts.description_th ?? '', banner.texts.description_en);
  if (desc) {
    const p = document.createElement('p');
    p.className = 'pdpa-cc-desc';
    p.textContent = desc;
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

  const acceptBtn = makeButton(text(banner.texts.accept_all_th, banner.texts.accept_all_en), false);
  acceptBtn.addEventListener('click', () => cb.onAcceptAll());

  const rejectBtn = makeButton(text(banner.texts.reject_all_th, banner.texts.reject_all_en), false);
  rejectBtn.addEventListener('click', () => cb.onRejectAll());

  const settingsBtn = makeButton(text(banner.texts.settings_th, banner.texts.settings_en), true);
  let settingsOpen = false;
  settingsBtn.addEventListener('click', () => {
    settingsOpen = !settingsOpen;
    catsEl.style.display = settingsOpen ? '' : 'none';
    if (settingsOpen) {
      const saveBtn = makeButton(text(banner.texts.accept_all_th, banner.texts.accept_all_en), true);
      saveBtn.textContent = text('บันทึกการตั้งค่า', 'Save settings');
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
  label.textContent = text(c.name_th, c.name_en);
  const descText = text(c.description_th ?? '', c.description_en);
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
