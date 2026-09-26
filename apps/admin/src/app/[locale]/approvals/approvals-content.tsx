"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@pdpa/ui";
import { createApiClient, useMyApprovals, useRecordVersion, useVersionMutations, type ApprovalInboxItem } from "@pdpa/api-client";
import { VersionDiff } from "@/components/version-diff";
import { RecordVersions } from "@/components/record-versions";

type Decision = "approved" | "returned" | "rejected";

function problemCode(e: unknown): string | undefined {
  return typeof e === "object" && e !== null && "code" in e ? String((e as { code: unknown }).code) : undefined;
}

export function ApprovalsContent() {
  const t = useTranslations("versions");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const inbox = useMyApprovals(client);
  const [selected, setSelected] = useState<ApprovalInboxItem>();
  const [done, setDone] = useState(false);

  return (
    <main className="mx-auto grid max-w-7xl gap-6 p-8 text-sm lg:grid-cols-[2fr_3fr]">
      <section className="space-y-3">
        <h1 className="text-xl font-semibold">{t("inbox.title")}</h1>
        {done && <p role="status" className="text-emerald-800">{t("inbox.done")}</p>}
        {inbox.isPending ? <p className="text-slate-500">{t("loading")}</p> : inbox.isError ? <p className="text-red-700">{t("loadError")}</p> : inbox.data.length === 0 ? (
          <p className="text-slate-500">{t("inbox.empty")}</p>
        ) : (
          <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
            {inbox.data.map((it) => (
              <li key={it.id}>
                <button className={`w-full px-3 py-2 text-left hover:bg-slate-50 ${selected?.id === it.id ? "bg-slate-50" : ""}`} onClick={() => { setSelected(it); setDone(false); }}>
                  <p className="font-medium">{t("inbox.item", { title: it.title ?? it.entity_type, no: it.version })}</p>
                  <p className="text-xs text-slate-500">{t("step", { step: it.step, role: it.role })} · {t("inbox.requestedBy", { name: it.requester_name ?? "" })}</p>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
      <section>
        {selected ? <Review key={selected.id} item={selected} onDone={() => { setSelected(undefined); setDone(true); }} client={client} /> : <p className="text-slate-500">{t("inbox.select")}</p>}
      </section>
    </main>
  );
}

function Review({ item, onDone, client }: { item: ApprovalInboxItem; onDone: () => void; client: ReturnType<typeof createApiClient> }) {
  const t = useTranslations("versions");
  const version = useRecordVersion(client, item.version_id);
  const m = useVersionMutations(client);
  const [reason, setReason] = useState("");
  const decide = (decision: Decision) => m.decide.mutate({ approvalId: item.id, rowVersion: item.row_version, decision, reason }, { onSuccess: onDone });
  const error = m.decide.error ? (problemCode(m.decide.error) === "versioning.self_approval" ? t("inbox.selfApproval") : t("actionError")) : null;

  return (
    <div className="space-y-4" data-testid="approval-review">
      <header>
        <h2 className="text-lg font-semibold">{t("inbox.item", { title: item.title ?? item.entity_type, no: item.version })}</h2>
        <p className="text-slate-600">{item.author_name && t("inbox.author", { name: item.author_name })} · {t("step", { step: item.step, role: item.role })}</p>
      </header>
      <div className="space-y-2">
        <h3 className="font-medium">{t("inbox.changes")}</h3>
        {version.data ? <VersionDiff changes={version.data.diff} /> : <p className="text-slate-500">{t("loading")}</p>}
      </div>
      <label className="block">
        <span className="block text-slate-600">{t("inbox.reason")}</span>
        <textarea className="mt-1 h-20 w-full rounded-md border border-slate-300 px-2 py-1" value={reason} onChange={(e) => setReason(e.target.value)} />
      </label>
      <div className="flex flex-wrap gap-2">
        <Button onClick={() => decide("approved")} disabled={m.decide.isPending}>{t("inbox.approve")}</Button>
        <Button variant="secondary" onClick={() => decide("returned")} disabled={m.decide.isPending || !reason.trim()}>{t("inbox.return")}</Button>
        <Button variant="secondary" onClick={() => decide("rejected")} disabled={m.decide.isPending || !reason.trim()}>{t("inbox.reject")}</Button>
      </div>
      {error && <p className="text-red-700" role="alert">{error}</p>}
      <RecordVersions entityType={item.entity_type} entityId={item.entity_id} canEdit={false} canPublish={false} />
    </div>
  );
}
