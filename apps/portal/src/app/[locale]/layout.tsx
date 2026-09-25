import type { ReactNode } from "react";
import { NextIntlClientProvider, hasLocale } from "next-intl";
import { notFound } from "next/navigation";
import { Sarabun } from "next/font/google";
import { locales } from "@pdpa/i18n";
import "../globals.css";

// Same typeface as the admin app (Thai government standard, covers Latin).
const sarabun = Sarabun({ subsets: ["thai", "latin"], weight: ["400", "500", "600", "700"], variable: "--font-sarabun" });

export const metadata = {
  title: "PDPA Platform — Portal",
};

export function generateStaticParams() {
  return locales.map((locale) => ({ locale }));
}

export default async function LocaleLayout({ children, params }: { children: ReactNode; params: Promise<{ locale: string }> }) {
  const { locale } = await params;
  if (!hasLocale(locales, locale)) notFound();
  return (
    <html lang={locale} className={sarabun.variable}>
      <body className="min-h-screen bg-slate-50 font-sans text-slate-900 antialiased">
        <NextIntlClientProvider>{children}</NextIntlClientProvider>
      </body>
    </html>
  );
}
