import type { ReactNode } from "react";
import { NextIntlClientProvider, hasLocale } from "next-intl";
import { getTranslations } from "next-intl/server";
import { notFound } from "next/navigation";
import { Sarabun } from "next/font/google";
import { locales } from "@pdpa/i18n";
import { LocaleSwitcher } from "@/components/locale-switcher";
import { NotificationBell } from "@/components/notification-bell";
import { auth } from "@/auth";
import { Providers } from "./providers";
import "../globals.css";

// Sarabun: the Thai government's standard typeface (used on official documents and forms), also
// covers Latin — one font for both locales instead of switching families per language.
const sarabun = Sarabun({
  subsets: ["thai", "latin"],
  weight: ["400", "500", "600", "700"],
  variable: "--font-sarabun",
});

export function generateStaticParams() {
  return locales.map((locale) => ({ locale }));
}

export default async function LocaleLayout({
  children,
  params,
}: {
  children: ReactNode;
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  if (!hasLocale(locales, locale)) notFound();

  const t = await getTranslations("common");
  const session = await auth();

  return (
    <html lang={locale} dir="ltr" className={sarabun.variable}>
      <body className="min-h-screen bg-slate-50 font-sans text-slate-900 antialiased">
        <NextIntlClientProvider>
          <Providers>
            <header className="flex items-center justify-between border-b border-slate-200 bg-white px-6 py-3">
              <span className="text-sm font-semibold">{t("appName")}</span>
              <div className="flex items-center gap-2">
                {session?.accessToken ? <NotificationBell /> : null}
                <LocaleSwitcher />
              </div>
            </header>
            {children}
          </Providers>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
