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

/** Swaps an inert text/plain placeholder for a real, executing <script> — shared by applyPreferences'
 * own mechanism 1 and guardDynamicScripts' pending-queue resolution below. */
function activatePlaceholder(placeholder: HTMLScriptElement): void {
  const real = document.createElement('script');
  for (const attr of Array.from(placeholder.attributes)) {
    if (attr.name === 'type' || attr.name === ACTIVATED_ATTR) continue;
    real.setAttribute(attr.name === 'data-pdpa-src' ? 'src' : attr.name, attr.value);
  }
  real.text = placeholder.text;
  placeholder.setAttribute(ACTIVATED_ATTR, 'true');
  placeholder.replaceWith(real);
}

export function applyPreferences(choices: Record<string, boolean>, rules: ScriptRule[] = [], root: ParentNode = document): void {
  // Mechanism 1: activate (or leave inert) declaratively-gated scripts.
  const pending = root.querySelectorAll<HTMLScriptElement>(`script[type="text/plain"][data-pdpa-category]:not([${ACTIVATED_ATTR}])`);
  pending.forEach((placeholder) => {
    const category = placeholder.getAttribute('data-pdpa-category') ?? '';
    if (categoryAllowed(choices, category)) activatePlaceholder(placeholder);
  });

  // Mechanism 2: remove still-present third-party scripts whose category is now disallowed.
  const loaded = Array.from(root.querySelectorAll<HTMLScriptElement>('script[src]'));
  const toBlock = scriptsToBlock(loaded.map((el) => ({ src: el.src })), rules, choices);
  for (const i of toBlock) loaded[i].remove();

  if (typeof window !== 'undefined' && typeof CustomEvent !== 'undefined') {
    window.dispatchEvent(new CustomEvent('pdpa:consent', { detail: { choices } }));
  }
}

const PENDING_ATTR = 'data-pdpa-pending';

// CON-03 ("Prior blocking"): CON-02's mechanism 2 above only sweeps the DOM once, at the moment a decision
// is applied — it can't catch a <script src> a tag manager (e.g. GTM itself, after *its own* container
// loads) injects *later*, during the window before any decision — or even the banner's own script_rules —
// has loaded yet. A MutationObserver was tried first and rejected: its callback fires as a microtask
// *after* the synchronous insertion that already starts the browser's fetch, so by the time it runs, the
// request (and in most engines, the script's eventual execution) can no longer be stopped — proven by a
// real failure in this package's own E2E test before this version. `guardDynamicScripts` instead patches
// `Node.prototype.appendChild`/`insertBefore` (the two ways a tag manager actually inserts a sub-tag) to
// intercept a <script src> *before* the original call ever runs — rewriting it in place to the same inert
// `type="text/plain"` placeholder shape a site owner's own declarative gating already uses (CLAUDE.md's
// own "rewrite type=text/plain" note), so the browser never fetches it at all.
//
// A script intercepted before this SDK even knows the tenant's real script_rules (the realistic case —
// the banner fetch is still an async network round-trip) is held in `pending` with no category yet; the
// returned `resolve(choices, rules)` must be called once those are known (and again on every later
// preference change) to classify each one: matching a block rule and still disallowed → stays inert with
// `data-pdpa-category` set, so applyPreferences' own mechanism 1 activates it exactly like a hand-authored
// placeholder once that category is allowed; matching no rule at all → not something a tenant ever flagged
// as tracking, so it is released immediately rather than held forever on a guess.
export function guardDynamicScripts(): { resolve: (choices: Record<string, boolean>, rules: ScriptRule[]) => void } {
  if (typeof Node === 'undefined' || typeof HTMLScriptElement === 'undefined') return { resolve: () => {} };
  const pending: HTMLScriptElement[] = [];

  function intercept(node: unknown): void {
    if (!(node instanceof HTMLScriptElement)) return;
    if (!node.src || node.hasAttribute(ACTIVATED_ATTR) || node.hasAttribute(PENDING_ATTR)) return;
    node.setAttribute(PENDING_ATTR, 'true');
    node.setAttribute('data-pdpa-src', node.src);
    node.removeAttribute('src');
    node.type = 'text/plain';
    pending.push(node);
  }

  for (const method of ['appendChild', 'insertBefore'] as const) {
    const original = Node.prototype[method] as (...args: never[]) => unknown;
    Node.prototype[method] = function (this: Node, ...args: never[]) {
      intercept(args[0]);
      return original.apply(this, args);
    } as never;
  }

  function resolve(choices: Record<string, boolean>, rules: ScriptRule[]): void {
    for (let i = pending.length - 1; i >= 0; i--) {
      const node = pending[i];
      const src = node.getAttribute('data-pdpa-src') ?? '';
      const rule = rules.find((r) => r.action === 'block' && matchesPattern(src, r.pattern));
      if (!rule) {
        activatePlaceholder(node);
        pending.splice(i, 1);
        continue;
      }
      node.setAttribute('data-pdpa-category', rule.category_code);
      if (categoryAllowed(choices, rule.category_code)) {
        activatePlaceholder(node);
        pending.splice(i, 1);
      }
      // else: left inert with data-pdpa-category now set — applyPreferences' own mechanism 1 will
      // activate it on a future decision the moment that category is allowed.
    }
  }

  return { resolve };
}
