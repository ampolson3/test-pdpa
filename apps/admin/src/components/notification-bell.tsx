"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { formatDate, type Locale } from "@pdpa/i18n";
import { createApiClient, useInbox, useMarkInboxRead, useUnreadCount } from "@pdpa/api-client";

const BFF = "/api/bff";

/** The in-app notification bell (PLT-04): unread count pushed over SSE, the latest messages on open. */
export function NotificationBell() {
  const t = useTranslations("notify");
  const locale = useLocale() as Locale;
  const client = useMemo(() => createApiClient(BFF), []);
  const unread = useUnreadCount(BFF);
  const [open, setOpen] = useState(false);
  const inbox = useInbox(client, open);
  const markRead = useMarkInboxRead(client);

  return (
    <div className="relative">
      <button
        type="button"
        aria-label={unread ? t("bell.labelUnread", { count: unread }) : t("bell.label")}
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="relative rounded-md p-2 hover:bg-slate-100"
      >
        <svg aria-hidden viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M18 8a6 6 0 1 0-12 0c0 7-3 9-3 9h18s-3-2-3-9M13.73 21a2 2 0 0 1-3.46 0" />
        </svg>
        {!!unread && (
          <span className="absolute -right-0.5 -top-0.5 min-w-4 rounded-full bg-red-600 px-1 text-center text-[10px] leading-4 text-white">
            {unread > 99 ? "99+" : unread}
          </span>
        )}
      </button>
      {open && (
        <div className="absolute right-0 z-10 mt-2 w-80 rounded-md border border-slate-200 bg-white shadow-lg">
          <p className="border-b border-slate-100 px-3 py-2 text-sm font-medium">{t("bell.title")}</p>
          {inbox.isPending ? (
            <p className="px-3 py-4 text-sm text-slate-500">{t("loading")}</p>
          ) : !inbox.data?.items.length ? (
            <p className="px-3 py-4 text-sm text-slate-500">{t("bell.empty")}</p>
          ) : (
            <ul className="max-h-96 divide-y divide-slate-100 overflow-y-auto">
              {inbox.data.items.map((m) => (
                <li key={m.id}>
                  <button
                    type="button"
                    className={`block w-full px-3 py-2 text-left text-sm hover:bg-slate-50 ${m.read ? "text-slate-500" : ""}`}
                    onClick={() => !m.read && markRead.mutate(m.id)}
                  >
                    {m.title && <span className={`block ${m.read ? "" : "font-medium"}`}>{m.title}</span>}
                    <span className="block">{m.body}</span>
                    <span className="block text-xs text-slate-400">{formatDate(m.created_at, locale, { month: "short", hour: "2-digit", minute: "2-digit" })}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
