import type { ReactNode } from "react";
import { NextIntlClientProvider, hasLocale } from "next-intl";
import { getTranslations } from "next-intl/server";
import { notFound } from "next/navigation";
import { Sarabun } from "next/font/google";
import { locales } from "@pdpa/i18n";
import { LocaleSwitcher } from "@/components/locale-switcher";
import "../globals.css";

// Same typeface as the admin app (Thai government standard, covers Latin).
const sarabun = Sarabun({ subsets: ["thai", "latin"], weight: ["400", "500", "600", "700"], variable: "--font-sarabun" });

export const metadata = {
  title: "PDPA Platform — Portal",
};

export function generateStaticParams() {
  return locales.map((locale) => ({ locale }));
}

// PLT-17's own acceptance criterion is about this shell + the home page: WCAG 2.1 AA and a fast load on
// 4G. The accessibility half needs a real page structure — a skip link ahead of everything else, a
// <header> banner landmark (not a <nav>: there's nothing to navigate yet beyond the locale switcher) and
// a <footer> contentinfo landmark — which a bare body never had. Each page under this layout renders its
// own <main> (the consent form, the notice page, the home page below), so this file never renders one
// itself — two <main> landmarks on one page would itself be an accessibility defect. The performance half
// needs no client JS here: this whole shell is a Server Component, and LocaleSwitcher is the only
// "use client" island on the page.
export default async function LocaleLayout({ children, params }: { children: ReactNode; params: Promise<{ locale: string }> }) {
  const { locale } = await params;
  if (!hasLocale(locales, locale)) notFound();
  const t = await getTranslations("common");
  const tp = await getTranslations("portal");
  return (
    <html lang={locale} dir="ltr" className={sarabun.variable}>
      <body className="flex min-h-screen flex-col bg-slate-50 font-sans text-slate-900 antialiased">
        <NextIntlClientProvider>
          <a
            href="#main-content"
            className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-white focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-slate-900 focus:shadow-lg focus:outline focus:outline-2 focus:outline-offset-2 focus:outline-blue-700"
          >
            {tp("skipToContent")}
          </a>
          <header className="border-b border-slate-200 bg-white">
            <div className="mx-auto flex max-w-3xl items-center justify-between px-4 py-3 sm:px-8">
              <span className="text-sm font-semibold">{t("appName")}</span>
              <LocaleSwitcher />
            </div>
          </header>
          {/* tabIndex=-1: the skip link above must move focus here, not just scroll to it
              (WCAG 2.4.1 Bypass Blocks) — a plain id target only scrolls for most browsers. */}
          <div id="main-content" tabIndex={-1} className="flex-1 focus:outline-none">
            {children}
          </div>
          <footer className="border-t border-slate-200 bg-white">
            <div className="mx-auto max-w-3xl px-4 py-4 text-xs text-slate-500 sm:px-8">{tp("footer")}</div>
          </footer>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
