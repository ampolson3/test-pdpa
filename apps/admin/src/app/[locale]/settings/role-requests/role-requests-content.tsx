"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import {
  createApiClient,
  useMentionSearch,
  useRequestRoleAssignment,
  useRoleAssignmentRequests,
  type RoleAssignmentRequest,
} from "@pdpa/api-client";
import { RecordVersions } from "@/components/record-versions";

const ENTITY_TYPE = "iam_role_assignment_request";

/**
 * IAM-06: ORGADMIN picks a user here and asks that they be granted ORGADMIN. The request is
 * immediately submitted for SEC's decision (the generic /approvals inbox) — this page never grants
 * anything itself. Once SEC approves, publishing (also here, gated on admin.role.approve) is the one
 * moment the role actually takes effect.
 */
export function RoleRequestsContent() {
  const t = useTranslations("roleRequests");
  const locale = useLocale() as Locale;
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const canRequest = usePermission("admin.user.update");
  const canPublish = usePermission("admin.role.approve");
  const list = useRoleAssignmentRequests(client);
  const [selected, setSelected] = useState<string>();
  const [query, setQuery] = useState<string | null>(null);
  const [picked, setPicked] = useState<{ id: string; display_name: string }>();
  const users = useMentionSearch(client, query);
  const m = useRequestRoleAssignment(client);

  const submit = () => {
    if (!picked) return;
    m.mutate(picked.id, { onSuccess: () => { setPicked(undefined); setQuery(null); } });
  };

  return (
    <main className="mx-auto max-w-4xl space-y-6 p-8 text-sm">
      <header>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-slate-600">{t("description")}</p>
      </header>

      {canRequest && (
        <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="role-request-form">
          <h2 className="font-medium">{t("requestTitle")}</h2>
          <div className="relative">
            <label className="block">
              <span className="block text-slate-600">{t("targetUser")}</span>
              <input className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1" value={picked ? picked.display_name : query ?? ""}
                onChange={(e) => { setPicked(undefined); setQuery(e.target.value || null); }} placeholder={t("searchPlaceholder")} />
            </label>
            {query && !picked && !!users.data?.length && (
              <ul role="listbox" className="absolute z-10 w-full rounded-md border border-slate-200 bg-white text-sm shadow">
                {users.data.map((u) => (
                  <li key={u.id} role="option" aria-selected={false}>
                    <button type="button" className="block w-full px-3 py-1.5 text-left hover:bg-slate-50" onClick={() => { setPicked(u); setQuery(null); }}>
                      {u.display_name}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
          <Button onClick={submit} disabled={!picked || m.isPending}>{t("submit")}</Button>
          {m.isError && <p className="text-red-700" role="alert">{t("submitError")}</p>}
        </section>
      )}

      <section className="space-y-2">
        <h2 className="font-medium">{t("historyTitle")}</h2>
        {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : list.data.length === 0 ? (
          <p className="text-slate-500">{t("empty")}</p>
        ) : (
          <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
            {list.data.map((r: RoleAssignmentRequest) => (
              <li key={r.id} className="space-y-1 px-3 py-2">
                <button className="w-full text-left" onClick={() => setSelected(selected === r.id ? undefined : r.id)}>
                  <p className="font-medium">{t("row", { role: r.role_code, user: r.target_user_name })}</p>
                  <p className="text-xs text-slate-500">
                    {t("requestedBy", { name: r.requested_by_name })} · {formatDate(r.created_at, locale, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}
                    {r.applied_at ? ` · ${t("applied")}` : ""}
                  </p>
                </button>
                {selected === r.id && (
                  <RecordVersions entityType={ENTITY_TYPE} entityId={r.id} canEdit={false} canPublish={canPublish} compare={false} />
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
    </main>
  );
}
