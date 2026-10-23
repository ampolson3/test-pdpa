"use client";

import { useTranslations } from "next-intl";
import type { VersionChange } from "@pdpa/api-client";

function show(v: unknown): string {
  if (v === undefined || v === null) return "—";
  return typeof v === "string" ? v : JSON.stringify(v);
}

/** A table of field-level changes between two versions (PLT-08). */
export function VersionDiff({ changes }: { changes: VersionChange[] }) {
  const t = useTranslations("versions");
  if (changes.length === 0) return <p className="text-slate-500">{t("noChanges")}</p>;
  return (
    <table className="w-full rounded-md border border-slate-200 bg-white text-sm" data-testid="version-diff">
      <thead className="bg-slate-50 text-left text-slate-600">
        <tr><th className="px-2 py-1">{t("path")}</th><th className="px-2 py-1">{t("before")}</th><th className="px-2 py-1">{t("after")}</th></tr>
      </thead>
      <tbody className="divide-y divide-slate-100">
        {changes.map((c) => (
          <tr key={c.path}>
            <td className="px-2 py-1 font-mono text-xs">{c.path}</td>
            <td className="px-2 py-1 text-red-800 line-through decoration-red-300">{show(c.before)}</td>
            <td className="px-2 py-1 text-emerald-800">{show(c.after)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
