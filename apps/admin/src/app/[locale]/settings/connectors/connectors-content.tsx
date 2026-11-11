"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useConnectorTypes,
  useConnectors,
  useSaveConnector,
  useDeleteConnector,
  useTestConnector,
  type Connector,
  type ConnectorTestResult,
} from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const STATUSES = ["draft", "active", "error", "disabled"] as const;

function detail(e: unknown): string {
  return typeof e === "object" && e !== null
    ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ")
    : "";
}

type Draft = {
  id?: string;
  rowVersion?: number;
  name: string;
  connectorType: string;
  configText: string;
  secret: string;
  capabilities: string[];
};

const blank: Draft = { name: "", connectorType: "", configText: "{}", secret: "", capabilities: [] };

function fromConnector(c: Connector): Draft {
  return {
    id: c.id,
    rowVersion: c.row_version,
    name: c.name,
    connectorType: c.connector_type,
    configText: JSON.stringify(c.config ?? {}, null, 2),
    secret: "", // write-only — never pre-filled, omitting leaves it unchanged
    capabilities: c.capabilities ?? [],
  };
}

/** PLT-20's own admin screen: register a connector against the process's live Registry (postgres/mysql/
 *  sqlserver/rest, this feature's own "ชุดแรก"), and test it from here — the literal acceptance
 *  criterion. A new connector_type registered in internal/wiring.Connectors() shows up in the picker
 *  with no change to this page. */
export function ConnectorsContent() {
  const t = useTranslations("connectors");
  const canRead = usePermission("ropa.discovery.read");
  const canCreate = usePermission("ropa.discovery.create");
  const canUpdate = usePermission("ropa.discovery.update");
  const canDelete = usePermission("ropa.discovery.delete");
  const canExecute = usePermission("ropa.discovery.execute");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [status, setStatus] = useState<string>("");
  const types = useConnectorTypes(client);
  const list = useConnectors(client, { status: status ? (status as never) : undefined });
  const save = useSaveConnector(client);
  const del = useDeleteConnector(client);
  const test = useTestConnector(client);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [testResults, setTestResults] = useState<Record<string, ConnectorTestResult>>({});

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const typeOptions = types.data ?? [];
  const selectedTypeCapabilities = typeOptions.find((ty) => ty.code === draft?.connectorType)?.capabilities ?? [];

  const submit = () => {
    if (!draft || !draft.name.trim() || !draft.connectorType) return;
    let config: Record<string, unknown>;
    try {
      config = JSON.parse(draft.configText || "{}");
    } catch {
      return;
    }
    save.mutate(
      {
        id: draft.id,
        rowVersion: draft.rowVersion,
        input: {
          name: draft.name,
          connector_type: draft.connectorType,
          config,
          secret: draft.secret === "" ? undefined : draft.secret,
          capabilities: draft.capabilities as never,
        },
      },
      { onSuccess: () => setDraft(null) },
    );
  };

  const runTest = (c: Connector) => {
    test.mutate(c.id, {
      onSuccess: (result) => setTestResults((prev) => ({ ...prev, [c.id]: result })),
    });
  };

  return (
    <main className="mx-auto max-w-5xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        {canCreate && (
          <Button onClick={() => { save.reset(); setDraft({ ...blank }); }} data-testid="new-connector">
            {t("newConnector")}
          </Button>
        )}
      </header>

      <div className="flex flex-wrap gap-4" data-testid="connector-filters">
        <label>
          <span className="block text-slate-600">{t("filters.status")}</span>
          <select className={INPUT} value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="">{t("filters.all")}</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>{t(`statuses.${s}`)}</option>
            ))}
          </select>
        </label>
      </div>

      {draft && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2" data-testid="connector-form">
          <legend className="px-1 font-semibold">{draft.id ? t("form.editTitle") : t("form.newTitle")}</legend>
          <label className="sm:col-span-2">
            <span className="block text-slate-600">{t("form.name")}</span>
            <input className={INPUT} value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
          </label>
          <label>
            <span className="block text-slate-600">{t("form.connectorType")}</span>
            <select
              className={INPUT}
              value={draft.connectorType}
              disabled={!!draft.id}
              onChange={(e) => setDraft({ ...draft, connectorType: e.target.value, capabilities: [] })}
            >
              <option value="">{t("filters.all")}</option>
              {typeOptions.map((ty) => (
                <option key={ty.code} value={ty.code}>{ty.code}</option>
              ))}
            </select>
          </label>
          <label>
            <span className="block text-slate-600">{t("form.secret")}</span>
            <input
              type="password"
              className={INPUT}
              placeholder={draft.id ? t("form.secretUnchanged") : ""}
              value={draft.secret}
              onChange={(e) => setDraft({ ...draft, secret: e.target.value })}
            />
          </label>
          <label className="sm:col-span-2">
            <span className="block text-slate-600">{t("form.config")}</span>
            <textarea
              className={`${INPUT} h-32 font-mono text-xs`}
              value={draft.configText}
              onChange={(e) => setDraft({ ...draft, configText: e.target.value })}
            />
          </label>
          {selectedTypeCapabilities.length > 0 && (
            <div className="sm:col-span-2">
              <span className="block text-slate-600">{t("form.capabilities")}</span>
              <div className="flex flex-wrap gap-3">
                {selectedTypeCapabilities.map((cap) => (
                  <label key={cap} className="flex items-center gap-1">
                    <input
                      type="checkbox"
                      checked={draft.capabilities.includes(cap)}
                      onChange={(e) =>
                        setDraft({
                          ...draft,
                          capabilities: e.target.checked
                            ? [...draft.capabilities, cap]
                            : draft.capabilities.filter((c) => c !== cap),
                        })
                      }
                    />
                    {cap}
                  </label>
                ))}
              </div>
            </div>
          )}

          {save.isError && (
            <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(save.error) })}</p>
          )}
          <div className="flex gap-2 sm:col-span-2">
            <Button onClick={submit} disabled={save.isPending || !draft.name.trim() || !draft.connectorType}>
              {t("form.save")}
            </Button>
            <Button variant="secondary" onClick={() => { save.reset(); setDraft(null); }}>{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      {list.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : list.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : rows.length === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : (
        <table className="w-full border-collapse text-left" data-testid="connectors-table">
          <thead>
            <tr className="border-b border-slate-200 text-slate-500">
              <th className="py-2">{t("table.name")}</th>
              <th className="py-2">{t("table.type")}</th>
              <th className="py-2">{t("table.status")}</th>
              <th className="py-2">{t("table.lastTested")}</th>
              <th className="py-2" />
            </tr>
          </thead>
          <tbody>
            {rows.map((c) => (
              <tr key={c.id} className="border-b border-slate-100">
                <td className="py-2">{c.name}</td>
                <td className="py-2">{c.connector_type}</td>
                <td className="py-2">{t(`statuses.${c.status}`)}</td>
                <td className="py-2">{c.last_tested_at ? new Date(c.last_tested_at).toLocaleString() : t("never")}</td>
                <td className="py-2 space-x-2">
                  {canExecute && (
                    <button className="text-sky-700 underline" onClick={() => runTest(c)} disabled={test.isPending}>
                      {t("test")}
                    </button>
                  )}
                  {canUpdate && (
                    <button className="text-sky-700 underline" onClick={() => { save.reset(); setDraft(fromConnector(c)); }}>
                      {t("edit")}
                    </button>
                  )}
                  {canDelete && (
                    <button
                      className="text-red-700 underline"
                      onClick={() => del.mutate({ id: c.id, rowVersion: c.row_version })}
                      disabled={del.isPending}
                    >
                      {t("delete")}
                    </button>
                  )}
                  {testResults[c.id] && (
                    <span className={testResults[c.id].ok ? "text-emerald-700" : "text-red-700"}>
                      {testResults[c.id].ok ? t("testOk") : t("testFailed", { message: testResults[c.id].message ?? "" })}
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {list.hasNextPage && (
        <Button variant="secondary" onClick={() => list.fetchNextPage()} disabled={list.isFetchingNextPage}>
          {t("loading")}
        </Button>
      )}
    </main>
  );
}
