import type { Category, ConsentAction, ScriptRule } from './types';

// CON-02's own acceptance criterion: "เปิด/ปิดรายหมวดแล้วมีผลทันทีกับสคริปต์ในหมวดนั้น" — toggling a
// category takes effect on its scripts immediately, no page reload. Two complementary mechanisms, the same
// ones every commercial consent-management platform uses (there is no way to literally un-execute code
// that already ran — a universal limitation, not one specific to this SDK):
//
// 1. Declarative gating: a site owner marks a script that must wait for consent as
//    <script type="text/plain" data-pdpa-category="analytics" data-pdpa-src="...">. While the category is
//    off, the browser never executes it (type="text/plain" is inert by spec). The moment the category
//    turns on, this SDK activates it — swaps in a real, executing <script>. Turning the category back off
//    after that can only stop what hasn't run yet; this SDK then leaves it inert (never re-activates).
// 2. Pattern-based removal: cookie.script_rules declares third-party script URL patterns per category.
//    Any matching <script src> tag still present (not yet loaded/dynamically inserted) is removed the
//    moment its category is disallowed, so it never loads.
//
// A `pdpa:consent` CustomEvent is dispatched on every change so a site's own code (already-running
// trackers it fully controls) can react — e.g. stop sending further analytics calls.

const ACTIVATED_ATTR = 'data-pdpa-activated';

export function choicesForAction(action: ConsentAction, categories: Category[], custom?: Record<string, boolean>): Record<string, boolean> {
  const choices: Record<string, boolean> = {};
  for (const c of categories) {
    if (action === 'accept_all') choices[c.code] = true;
    else if (action === 'reject_all') choices[c.code] = c.is_required;
    else choices[c.code] = c.is_required || !!custom?.[c.code];
  }
  return choices;
}

export function categoryAllowed(choices: Record<string, boolean>, code: string): boolean {
  return !!choices[code];
}

/** Substring match by default; a pattern wrapped in slashes (/.../) is a regex — the tenant's own choice. */
export function matchesPattern(src: string, pattern: string): boolean {
  if (pattern.length > 1 && pattern.startsWith('/') && pattern.endsWith('/')) {
    try {
      return new RegExp(pattern.slice(1, -1)).test(src);
    } catch {
      return false;
    }
  }
  return src.includes(pattern);
}

/** Which currently-loaded <script src> elements must be removed given the current choices. */
export function scriptsToBlock(scripts: { src: string }[], rules: ScriptRule[], choices: Record<string, boolean>): number[] {
  const indexes: number[] = [];
  scripts.forEach((s, i) => {
    for (const rule of rules) {
      if (rule.action !== 'block') continue;
      if (categoryAllowed(choices, rule.category_code)) continue;
      if (matchesPattern(s.src, rule.pattern)) {
        indexes.push(i);
        return;
      }
    }
  });
  return indexes;
}

export function applyPreferences(choices: Record<string, boolean>, rules: ScriptRule[] = [], root: ParentNode = document): void {
  // Mechanism 1: activate (or leave inert) declaratively-gated scripts.
  const pending = root.querySelectorAll<HTMLScriptElement>(`script[type="text/plain"][data-pdpa-category]:not([${ACTIVATED_ATTR}])`);
  pending.forEach((placeholder) => {
    const category = placeholder.getAttribute('data-pdpa-category') ?? '';
    if (!categoryAllowed(choices, category)) return;
    const real = document.createElement('script');
    for (const attr of Array.from(placeholder.attributes)) {
      if (attr.name === 'type' || attr.name === ACTIVATED_ATTR) continue;
      real.setAttribute(attr.name === 'data-pdpa-src' ? 'src' : attr.name, attr.value);
    }
    real.text = placeholder.text;
    placeholder.setAttribute(ACTIVATED_ATTR, 'true');
    placeholder.replaceWith(real);
  });

  // Mechanism 2: remove still-present third-party scripts whose category is now disallowed.
  const loaded = Array.from(root.querySelectorAll<HTMLScriptElement>('script[src]'));
  const toBlock = scriptsToBlock(loaded.map((el) => ({ src: el.src })), rules, choices);
  for (const i of toBlock) loaded[i].remove();

  if (typeof window !== 'undefined' && typeof CustomEvent !== 'undefined') {
    window.dispatchEvent(new CustomEvent('pdpa:consent', { detail: { choices } }));
  }
}
