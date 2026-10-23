import { describe, expect, it } from 'vitest';
import { resolveCopy } from './banner';
import type { BannerTexts } from './types';

const baseTexts: BannerTexts = {
  title_th: 'เราใช้คุกกี้', title_en: 'We use cookies',
  accept_all_th: 'ยอมรับทั้งหมด', accept_all_en: 'Accept all',
  reject_all_th: 'ปฏิเสธทั้งหมด', reject_all_en: 'Reject all',
  settings_th: 'ตั้งค่า', settings_en: 'Settings',
};

describe('resolveCopy (CON-06)', () => {
  it('falls back to th when the browser has no translation for its language', () => {
    expect(resolveCopy(baseTexts, ['fr-FR'], '').title).toBe('เราใช้คุกกี้');
  });

  it('uses en when the document lang is en and no extra translation exists', () => {
    expect(resolveCopy(baseTexts, ['en-US'], 'en').title).toBe('We use cookies');
  });

  it("picks a tenant's own extra-language translation when the browser prefers it — the acceptance criterion", () => {
    const texts: BannerTexts = { ...baseTexts, translations: { zh: { title: '我们使用Cookie', accept_all: '接受全部', reject_all: '拒绝全部', settings: '设置' } } };
    const copy = resolveCopy(texts, ['zh-CN', 'en-US'], '');
    expect(copy.title).toBe('我们使用Cookie');
    expect(copy.accept_all).toBe('接受全部');
  });

  it('never lets an extra translation override th/en — the browser must actually prefer it first', () => {
    const texts: BannerTexts = { ...baseTexts, translations: { zh: { title: '我们使用Cookie', accept_all: '接受全部', reject_all: '拒绝全部', settings: '设置' } } };
    // Browser prefers en, which already has its own dedicated field — the zh translation is never consulted.
    expect(resolveCopy(texts, ['en-US'], '').title).toBe('เราใช้คุกกี้');
  });

  it('matches a translation by primary language subtag even with a region suffix', () => {
    const texts: BannerTexts = { ...baseTexts, translations: { zh: { title: '我们使用Cookie', accept_all: '接受全部', reject_all: '拒绝全部', settings: '设置' } } };
    expect(resolveCopy(texts, ['zh-TW'], '').title).toBe('我们使用Cookie');
  });
});
