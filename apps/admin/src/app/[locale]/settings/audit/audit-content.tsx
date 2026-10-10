"use client";

import { Fragment, useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import { auditExportHref, createApiClient, useAuditLog, useMentionSearch, useVerifyAuditLog, type AuditEntry, type AuditFilter } from "@pdpa/api-client";

const BFF = "/api/bff";
const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

/** A date input's value (YYYY-MM-DD) as midnight Bangkok time in RFC 3339 (the platform's display zone). */
function bangkokMidnight(date: string): string | undefined {
  return date ? new Date(`${date}T00:00:00+07:00`).toISOString() : undefined;
}

export function AuditContent() {
  const t = useTranslations("auditLog");
  const locale = useLocale() as Locale;
  const canRead = usePermission("admin.audit.read");
  const canExport = usePermission("admin.audit.export");
  const client = useMemo(() => createApiClient(BFF), []);
  const [actor, setActor] = useState<{ id: string; name: string }>();
  const [actorQ, setActorQ] = useState("");
  const [form, setForm] = useState({ action_prefix: "", entity_type: "", entity_id: "", from: "", to: "", kind: "changes" as NonNullable<AuditFilter["kind"]> });
  const filter: AuditFilter = {
    actor_id: actor?.id, action_prefix: form.action_prefix.trim(), entity_type: form.entity_type.trim(), entity_id: form.entity_id.trim(),
    from: bangkokMidnight(form.from), to: bangkokMidnight(form.to), kind: form.kind,
  };
  const log = useAuditLog(client, filter);
  const people = useMentionSearch(client, actorQ.trim() || null);
  const verify = useVerifyAuditLog(client);
  const [open, setOpen] = useState<number>();

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;
  const entries = log.data?.pages.flatMap((p) => p.data) ?? [];
  const date = (iso: string) => formatDate(iso, locale, { month: "short", hour: "2-digit", minute: "2-digit", second: "2-digit" });

  return (
    <main className="mx-auto max-w-7xl space-y-4 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        <div className="flex gap-2">
          {canExport && <a className="rounded-md border border-slate-300 bg-white px-3 py-2 hover:bg-slate-50" href={auditExportHref(BFF, filter)}>{t("export")}</a>}
          <Button variant="secondary" onClick={() => verify.mutate()} disabled={verify.isPending}>{t("verify")}</Button>
        </div>
      </header>
      {verify.data && (
        <p role="status" className={verify.data.ok ? "text-emerald-800" : "font-medium text-red-700"}>
          {verify.data.ok ? t("verifyOk", { count: verify.data.checked }) : t("verifyBroken", { id: verify.data.broken_at_id ?? 0, reason: verify.data.reason ?? "" })}
        </p>
      )}
      {verify.isError && <p className="text-red-700">{t("verifyError")}</p>}

      <section className="grid gap-3 rounded-md border border-slate-200 bg-white p-3 sm:grid-cols-4">
        <div className="relative">
          <span className="block text-slate-600">{t("filters.actor")}</span>
          {actor ? (
            <p className="mt-1 flex items-center gap-2"><span className="font-medium" data-testid="actor-filter">{actor.name}</span><button className="text-xs underline" onClick={() => setActor(undefined)}>{t("filters.clear")}</button></p>
          ) : (
            <input className={INPUT} placeholder={t("filters.searchActor")} value={actorQ} onChange={(e) => setActorQ(e.target.value)} />
          )}
          {!actor && actorQ.trim() && (people.data?.length ?? 0) > 0 && (
            <ul role="listbox" className="absolute z-10 mt-1 w-full rounded-md border border-slate-200 bg-white shadow">
              {people.data!.map((u) => (
                <li key={u.id} role="option" aria-selected={false}><button className="w-full px-2 py-1 text-left hover:bg-slate-50" onClick={() => { setActor({ id: u.id, name: u.display_name }); setActorQ(""); }}>{u.display_name}</button></li>
              ))}
            </ul>
          )}
        </div>
        <label><span className="block text-slate-600">{t("filters.module")}</span><input className={`${INPUT} font-mono`} placeholder="iam." value={form.action_prefix} onChange={(e) => setForm({ ...form, action_prefix: e.target.value })} /></label>
        <label><span className="block text-slate-600">{t("filters.entityType")}</span><input className={`${INPUT} font-mono`} placeholder="user" value={form.entity_type} onChange={(e) => setForm({ ...form, entity_type: e.target.value })} /></label>
        <label><span className="block text-slate-600">{t("filters.entityId")}</span><input className={`${INPUT} font-mono`} value={form.entity_id} onChange={(e) => setForm({ ...form, entity_id: e.target.value })} /></label>
        <label><span className="block text-slate-600">{t("filters.from")}</span><input type="date" className={INPUT} value={form.from} onChange={(e) => setForm({ ...form, from: e.target.value })} /></label>
        <label><span className="block text-slate-600">{t("filters.to")}</span><input type="date" className={INPUT} value={form.to} onChange={(e) => setForm({ ...form, to: e.target.value })} /></label>
        <label><span className="block text-slate-600">{t("filters.kind")}</span>
          <select className={INPUT} value={form.kind} onChange={(e) => setForm({ ...form, kind: e.target.value as typeof form.kind })}>
            {(["changes", "requests", "all"] as const).map((k) => <option key={k} value={k}>{t(`filters.kinds.${k}`)}</option>)}
          </select>
        </label>
      </section>

      {log.isPending ? <p className="text-slate-500">{t("loading")}</p> : log.isError ? <p className="text-red-700">{t("loadError")}</p> : entries.length === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr><th className="px-3 py-2">{t("col.time")}</th><th className="px-3 py-2">{t("col.actor")}</th><th className="px-3 py-2">{t("col.action")}</th><th className="px-3 py-2">{t("col.entity")}</th><th className="px-3 py-2">{t("col.details")}</th></tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {entries.map((e) => (
              <Fragment key={e.id}>
                <tr data-testid="audit-row">
                  <td className="whitespace-nowrap px-3 py-2">{date(e.occurred_at)}</td>
                  <td className="px-3 py-2">{e.actor_name || (e.actor_type === "system" ? t("system") : e.actor_type)}</td>
                  <td className="px-3 py-2 font-mono text-xs">{e.action}</td>
                  <td className="px-3 py-2">{entityText(e)}</td>
                  <td className="px-3 py-2">{(e.before != null || e.after != null || e.ip) && (
                    <button className="underline" onClick={() => setOpen(open === e.id ? undefined : e.id)}>{open === e.id ? t("hide") : t("show")}</button>
                  )}</td>
                </tr>
                {open === e.id && (
                  <tr className="bg-slate-50">
                    <td colSpan={5} className="px-3 py-2">
                      <div className="grid gap-3 md:grid-cols-2">
                        <Json label={t("before")} value={e.before} />
                        <Json label={t("after")} value={e.after} />
                      </div>
                      {e.ip && <p className="mt-2 text-xs text-slate-500">{t("ip")}: {e.ip} · {e.user_agent}</p>}
                    </td>
                  </tr>
                )}
              </Fragment>
            ))}
          </tbody>
        </table>
      )}
      {log.hasNextPage && <Button variant="secondary" onClick={() => log.fetchNextPage()} disabled={log.isFetchingNextPage}>{t("more")}</Button>}
    </main>
  );
}

function entityText(e: AuditEntry): string {
  if (!e.entity_type) return "";
  return e.entity_name ? `${e.entity_type}: ${e.entity_name}` : `${e.entity_type}${e.entity_id ? ` · ${e.entity_id.slice(0, 8)}` : ""}`;
}

function Json({ label, value }: { label: string; value: unknown }) {
  return (
    <div>
      <p className="text-xs font-medium text-slate-600">{label}</p>
      <pre className="mt-1 max-h-64 overflow-auto rounded bg-white p-2 text-xs">{value == null ? "—" : JSON.stringify(value, null, 2)}</pre>
    </div>
  );
}
