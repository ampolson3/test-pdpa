"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useConnectors,
  useDiscoveryScans,
  useRunDiscoveryScan,
  useDiscoveryFindings,
  useReviewDiscoveryFinding,
} from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

function detail(e: unknown): string {
  return typeof e === "object" && e !== null
    ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ")
    : "";
}

export function DiscoveryContent() {
  const t = useTranslations("discovery");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const canRead = usePermission("ropa.discovery.read");
  const canRun = usePermission("ropa.discovery.execute");
  const canReview = usePermission("ropa.discovery.update");
  const connectors = useConnectors(client, { status: "active" });
  const scans = useDiscoveryScans(client);
  const run = useRunDiscoveryScan(client);
  const review = useReviewDiscoveryFinding(client);
  const [connectorId, setConnectorId] = useState("");
  const [openScan, setOpenScan] = useState<string | undefined>();
  const findings = useDiscoveryFindings(client, openScan);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;
  const list = connectors.data?.pages.flatMap((p) => p.data) ?? [];
  const nameOf = (id: string) => list.find((c) => c.id === id)?.name ?? id.slice(0, 8);

  return (
    <main className="mx-auto max-w-5xl space-y-6 p-8">
      <header>
        <h1 className="text-2xl font-semibold">{t("title")}</h1>
        <p className="text-sm text-slate-600">{t("intro")}</p>
      </header>

      {canRun && (
        <section className="rounded-md border border-slate-200 p-4">
          <label className="block text-sm">
            {t("connector")}
            <select className={INPUT} value={connectorId} onChange={(e) => setConnectorId(e.target.value)}>
              <option value="">{t("choose")}</option>
              {list.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>
          <Button
            className="mt-3"
            disabled={!connectorId || run.isPending}
            onClick={() => run.mutate({ connector_id: connectorId }, { onSuccess: (s) => setOpenScan(s.id) })}
          >
            {run.isPending ? t("running") : t("run")}
          </Button>
          {run.isError && <p className="mt-2 text-sm text-red-700">{t("error", { detail: detail(run.error) })}</p>}
          <p className="mt-2 text-xs text-slate-500">{t("privacy")}</p>
        </section>
      )}

      <section>
        <h2 className="mb-2 text-lg font-medium">{t("history")}</h2>
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b text-slate-600">
              <th className="py-1">{t("connector")}</th>
              <th>{t("status")}</th>
              <th>{t("objects")}</th>
              <th>{t("findings")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(scans.data ?? []).map((s) => (
              <tr key={s.id} className="border-b">
                <td className="py-1">{nameOf(s.connector_id)}</td>
                <td>{s.status}</td>
                <td>{s.objects_scanned}</td>
                <td>{s.findings_count}</td>
                <td>
                  <button className="text-blue-700 underline" onClick={() => setOpenScan(openScan === s.id ? undefined : s.id)}>
                    {t("viewFindings")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {scans.data?.length === 0 && <p className="mt-2 text-sm text-slate-500">{t("empty")}</p>}
      </section>

      {openScan && (
        <section>
          <h2 className="mb-2 text-lg font-medium">{t("findingsTitle")}</h2>
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b text-slate-600">
                <th className="py-1">{t("column")}</th>
                <th>{t("classifier")}</th>
                <th>{t("ratio")}</th>
                <th>{t("samples")}</th>
                <th>{t("status")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(findings.data ?? []).map((f) => (
                <tr key={f.id} className="border-b">
                  <td className="py-1 font-mono">{f.object_path}</td>
                  <td>{f.classifier_name}</td>
                  <td>{Math.round(f.match_ratio * 100)}%</td>
                  <td>{f.sample_count}</td>
                  <td>{f.status}</td>
                  <td className="space-x-2">
                    {canReview && f.status === "proposed" && (
                      <>
                        <button
                          className="text-green-700 underline"
                          onClick={() => review.mutate({ id: f.id, rowVersion: f.row_version, decision: "accepted" })}
                        >
                          {t("accept")}
                        </button>
                        <button
                          className="text-red-700 underline"
                          onClick={() => review.mutate({ id: f.id, rowVersion: f.row_version, decision: "rejected" })}
                        >
                          {t("reject")}
                        </button>
                      </>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </main>
  );
}
