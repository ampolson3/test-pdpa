import { describe, expect, it } from "vitest";
import { IntlMessageFormat } from "intl-messageformat";
import en from "./locales/en.json";
import th from "./locales/th.json";

type Tree = { [k: string]: string | Tree };

function flatten(tree: Tree, prefix = ""): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(tree)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (typeof v === "string") out[key] = v;
    else Object.assign(out, flatten(v, key));
  }
  return out;
}

const locales = { en: flatten(en as Tree), th: flatten(th as Tree) };

describe("message catalogs", () => {
  it("have the same keys in th and en", () => {
    expect(Object.keys(locales.th).sort()).toEqual(Object.keys(locales.en).sort());
  });

  // next-intl parses messages as ICU MessageFormat at render time, so a stray "{" (e.g. template
  // syntax like {{.name}} shown to the user) only fails in the browser. Parse them all here instead.
  for (const [locale, messages] of Object.entries(locales)) {
    it(`are all valid ICU messages (${locale})`, () => {
      for (const [key, msg] of Object.entries(messages)) {
        expect(() => new IntlMessageFormat(msg, locale), `${locale}:${key}`).not.toThrow();
      }
    });
  }

  it("keys contain no dots (next-intl reads them as nesting)", () => {
    for (const tree of [en, th] as Tree[]) {
      const walk = (t: Tree) => {
        for (const [k, v] of Object.entries(t)) {
          expect(k.includes("."), k).toBe(false);
          if (typeof v !== "string") walk(v);
        }
      };
      walk(tree);
    }
  });
});
