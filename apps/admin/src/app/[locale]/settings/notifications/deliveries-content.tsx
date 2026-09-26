"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import { createApiClient, useNotificationDeliveries, type NotificationChannel, type NotificationStatus } from "@pdpa/api-client";

const STATUSES: NotificationStatus[] = ["queued", "sent", "delivered", "failed", "cancelled"];
const CHANNELS: NotificationChannel[] = ["email", "sms", "line", "in_app"];
const statusStyle: Record<NotificationStatus, string> = {
  queued: "bg-slate-100 text-slate-700",
  sent: "bg-blue-100 text-blue-800",
  delivered: "bg-green-100 text-green-800",
  failed: "bg-red-100 text-red-800",
  cancelled: "bg-slate-100 text-slate-500",
};
const timeFormat: Intl.DateTimeFormatOptions = { month: "short", hour: "2-digit", minute: "2-digit", second: "2-digit" };

export function DeliveriesContent() {
  const t = useTranslations("notify");
  const locale = useLocale() as Locale;
  const allowed = usePermission("admin.notification.read");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [status, setStatus] = useState<NotificationStatus | "">("");
  const [channel, setChannel] = useState<NotificationChannel | "">("");
  const query = useNotificationDeliveries(client, { status: status || undefined, channel: channel || undefined });
  const rows = query.data?.pages.flatMap((p) => p.data) ?? [];

  if (!allowed) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  return (
    <main className="mx-auto max-w-6xl space-y-4 p-8">
      <header>
        <h1 className="text-xl font-semibold">{t("deliveries.title")}</h1>
        <p className="text-sm text-slate-600">{t("deliveries.description")}</p>
      </header>
      <div className="flex flex-wrap gap-3 text-sm">
        <select aria-label={t("deliveries.status")} className="rounded-md border border-slate-300 bg-white px-2 py-1" value={status} onChange={(e) => setStatus(e.target.value as NotificationStatus | "")}>
          <option value="">{t("deliveries.allStatuses")}</option>
          {STATUSES.map((s) => (
            <option key={s} value={s}>{t(`status.${s}`)}</option>
          ))}
        </select>
        <select aria-label={t("templates.channel")} className="rounded-md border border-slate-300 bg-white px-2 py-1" value={channel} onChange={(e) => setChannel(e.target.value as NotificationChannel | "")}>
          <option value="">{t("deliveries.allChannels")}</option>
          {CHANNELS.map((c) => (
            <option key={c} value={c}>{t(`channel.${c}`)}</option>
          ))}
        </select>
      </div>
      {query.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : query.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : rows.length === 0 ? (
        <p className="text-slate-500">{t("deliveries.empty")}</p>
      ) : (
        <div className="overflow-x-auto rounded-md border border-slate-200 bg-white">
          <table className="w-full text-left text-sm">
            <thead className="bg-slate-50 text-slate-600">
              <tr>
                <th className="px-3 py-2 font-medium">{t("deliveries.template")}</th>
                <th className="px-3 py-2 font-medium">{t("templates.channel")}</th>
                <th className="px-3 py-2 font-medium">{t("deliveries.recipient")}</th>
                <th className="px-3 py-2 font-medium">{t("deliveries.status")}</th>
                <th className="px-3 py-2 font-medium">{t("deliveries.attempts")}</th>
                <th className="px-3 py-2 font-medium">{t("deliveries.createdAt")}</th>
                <th className="px-3 py-2 font-medium">{t("deliveries.lastError")}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((d) => (
                <tr key={d.id} className="border-t border-slate-100 align-top">
                  <td className="px-3 py-2 font-mono text-xs">{d.template_code ?? "—"}</td>
                  <td className="px-3 py-2">{t(`channel.${d.channel}`)}</td>
                  <td className="px-3 py-2 font-mono text-xs">{d.recipient_masked ?? (d.recipient_user_id ? t("deliveries.user") : "—")}</td>
                  <td className="px-3 py-2">
                    <span className={`rounded-full px-2 py-0.5 text-xs ${statusStyle[d.status]}`}>{t(`status.${d.status}`)}</span>
                  </td>
                  <td className="px-3 py-2 tabular-nums">{d.attempts}</td>
                  <td className="px-3 py-2 whitespace-nowrap">{formatDate(d.created_at, locale, timeFormat)}</td>
                  <td className="max-w-xs truncate px-3 py-2 text-slate-600" title={d.error ?? undefined}>{d.error ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {query.hasNextPage && (
        <Button variant="secondary" onClick={() => query.fetchNextPage()} disabled={query.isFetchingNextPage}>
          {t("loadMore")}
        </Button>
      )}
    </main>
  );
}
