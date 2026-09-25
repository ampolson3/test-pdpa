import type { ReactNode } from "react";
import "./globals.css";

export const metadata = {
  title: "PDPA Platform — Portal",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="th">
      <body className="min-h-screen bg-slate-50 text-slate-900 antialiased">{children}</body>
    </html>
  );
}
