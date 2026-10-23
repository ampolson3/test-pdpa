import { describe, expect, it } from 'vitest';
import { categoryAllowed, choicesForAction, matchesPattern, scriptsToBlock } from './scripts';
import type { Category, ScriptRule } from './types';

const categories: Category[] = [
  { id: 'c1', code: 'necessary', name_th: 'จำเป็น', is_required: true, display_order: 0 },
  { id: 'c2', code: 'analytics', name_th: 'วิเคราะห์', is_required: false, display_order: 1 },
  { id: 'c3', code: 'marketing', name_th: 'การตลาด', is_required: false, display_order: 2 },
];

describe('choicesForAction', () => {
  it('accept_all turns every category on', () => {
    const c = choicesForAction('accept_all', categories);
    expect(c).toEqual({ necessary: true, analytics: true, marketing: true });
  });

  it('reject_all keeps only required categories on — the acceptance criterion\'s own one-click reject', () => {
    const c = choicesForAction('reject_all', categories);
    expect(c).toEqual({ necessary: true, analytics: false, marketing: false });
  });

  it('custom respects the given map but always forces required categories on', () => {
    const c = choicesForAction('custom', categories, { analytics: true, marketing: false, necessary: false });
    expect(c).toEqual({ necessary: true, analytics: true, marketing: false });
  });
});

describe('categoryAllowed', () => {
  it('is false for an absent key, not undefined-truthy', () => {
    expect(categoryAllowed({}, 'analytics')).toBe(false);
    expect(categoryAllowed({ analytics: true }, 'analytics')).toBe(true);
  });
});

describe('matchesPattern', () => {
  it('matches a plain substring', () => {
    expect(matchesPattern('https://www.googletagmanager.com/gtag/js', 'googletagmanager.com')).toBe(true);
    expect(matchesPattern('https://example.com/app.js', 'googletagmanager.com')).toBe(false);
  });

  it('matches a /regex/ pattern', () => {
    expect(matchesPattern('https://cdn.example.com/pixel-123.js', '/pixel-\\d+\\.js/')).toBe(true);
    expect(matchesPattern('https://cdn.example.com/other.js', '/pixel-\\d+\\.js/')).toBe(false);
  });

  it('never throws on an invalid regex pattern', () => {
    expect(matchesPattern('https://example.com', '/[/')).toBe(false);
  });
});

describe('scriptsToBlock', () => {
  const rules: ScriptRule[] = [
    { pattern: 'googletagmanager.com', category_code: 'analytics', action: 'block' },
    { pattern: 'facebook.net', category_code: 'marketing', action: 'block' },
  ];
  const scripts = [
    { src: 'https://www.googletagmanager.com/gtag/js' },
    { src: 'https://connect.facebook.net/signals.js' },
    { src: 'https://example.com/app.js' },
  ];

  it('blocks only scripts whose category is disallowed — the acceptance criterion\'s own immediate effect', () => {
    expect(scriptsToBlock(scripts, rules, { analytics: false, marketing: false })).toEqual([0, 1]);
    expect(scriptsToBlock(scripts, rules, { analytics: true, marketing: false })).toEqual([1]);
    expect(scriptsToBlock(scripts, rules, { analytics: true, marketing: true })).toEqual([]);
  });

  it('an allow-action rule never blocks', () => {
    const allow: ScriptRule[] = [{ pattern: 'example.com', category_code: 'necessary', action: 'allow' }];
    expect(scriptsToBlock(scripts, allow, {})).toEqual([]);
  });
});
