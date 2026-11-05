"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useDsarIntakeForms,
  useDsarIntakeFormMutations,
  useDsarTrackingPortalLink,
  useDsarTrackingPortalMutations,
  useLegalEntities,
  type DsarIntakeForm,
} from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

export function DsarFormsContent({ portalUrl }: { portalUrl: string }) {
  const t = useTranslations("dsarForms");
  const locale = useLocale();
  const canRead = usePermission("dsar.form.read");
  const canCreate = usePermission("dsar.form.create");
  const canPublish = usePermission("dsar.form.publish");
  const canRetire = usePermission("dsar.form.delete");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const list = useDsarIntakeForms(client);
  const legalEntities = useLegalEntities(client);
  const m = useDsarIntakeFormMutations(client);
  const [name, setName] = useState("");
  const [legalEntityId, setLegalEntityId] = useState("");
  const [error, setError] = useState("");

  if (!canRead) return <main className="mx-auto max-w-4xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const entities = legalEntities.data ?? [];

  const create = () => {
    if (!name.trim() || !legalEntityId) return;
    setError("");
    m.create.mutate({ name: name.trim(), legal_entity_id: legalEntityId }, {
      onSuccess: () => { setName(""); setLegalEntityId(""); },
      onError: (e) => setError(detail(e)),
    });
  };

  return (
    <main className="mx-auto max-w-4xl space-y-6 p-4 sm:p-8">
      <h1 className="text-xl font-semibold">{t("title")}</h1>
      <p className="text-sm text-slate-600">{t("intro")}</p>

      {canCreate && (
        <section className="space-y-2 rounded-lg border border-slate-200 bg-white p-4">
          <h2 className="font-medium">{t("createTitle")}</h2>
          <label className="block text-sm">
            {t("name")}
            <input value={name} onChange={(e) => setName(e.target.value)} className={INPUT} data-testid="form-name" />
          </label>
          <label className="block text-sm">
            {t("legalEntity")}
            <select value={legalEntityId} onChange={(e) => setLegalEntityId(e.target.value)} className={INPUT} data-testid="form-legal-entity">
              <option value="">{t("legalEntityPlaceholder")}</option>
              {entities.map((e) => (
                <option key={e.id} value={e.id}>{e.name_th}</option>
              ))}
            </select>
          </label>
          {error && <p className="text-sm text-rose-700" role="alert">{error}</p>}
          <button type="button" onClick={create} disabled={m.create.isPending} className="rounded-md bg-blue-700 px-3 py-1.5 text-sm text-white disabled:opacity-60" data-testid="create-form">
            {t("create")}
          </button>
        </section>
      )}

      <table className="w-full text-sm">
        <thead className="text-left text-slate-500">
          <tr>
            <th className="py-1">{t("name")}</th>
            <th>{t("status")}</th>
            <th>{t("publicLink")}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {rows.map((f) => (
            <Row key={f.id} f={f} portalUrl={portalUrl} locale={locale} canPublish={canPublish} canRetire={canRetire}
              onPublish={() => m.publish.mutate(f)} onRetire={() => m.retire.mutate(f)} />
          ))}
        </tbody>
      </table>
      {rows.length === 0 && <p className="text-sm text-slate-500">{t("empty")}</p>}

      <TrackingPortalPanel portalUrl={portalUrl} entities={entities} />
    </main>
  );
}

// DSAR-19: one self-service status-tracking link per legal entity — any of a tenant's own published
// tracking links works for any of that tenant's requests, so this mirrors CON-18's own preference-centre
// panel exactly rather than being tied to one intake form.
function TrackingPortalPanel({ portalUrl, entities }: { portalUrl: string; entities: { id: string; name_th: string }[] }) {
  const t = useTranslations("dsarForms");
  const locale = useLocale();
  const canPublish = usePermission("dsar.form.publish");
  const canRetire = usePermission("dsar.form.delete");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [entityId, setEntityId] = useState("");
  const chosen = entityId || entities[0]?.id || "";
  const link = useDsarTrackingPortalLink(client, chosen || undefined);
  const m = useDsarTrackingPortalMutations(client);
  const url = link.data?.public_key ? `${portalUrl.replace(/\/$/, "")}/${locale}/dsar-status/${link.data.public_key}` : "";

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="tracking-portal-panel">
      <div>
        <h2 className="font-medium">{t("tracking.title")}</h2>
        <p className="text-sm text-slate-600">{t("tracking.intro")}</p>
      </div>
      <label className="block max-w-sm text-sm">
        {t("legalEntity")}
        <select className={INPUT} value={chosen} onChange={(e) => setEntityId(e.target.value)} data-testid="tracking-legal-entity">
          {entities.map((e) => <option key={e.id} value={e.id}>{e.name_th}</option>)}
        </select>
      </label>
      {link.data?.public_key ? (
        <div className="space-y-2">
          <div>
            <p className="font-medium text-sm">{t("tracking.hostedLink")}</p>
            <p className="flex flex-wrap items-center gap-2">
              <a className="break-all text-sm text-sky-700 underline" href={url} target="_blank" rel="noreferrer" data-testid="tracking-link">{url}</a>
            </p>
          </div>
          {canRetire && (
            <Button variant="secondary" onClick={() => m.retire.mutate(chosen)} disabled={m.retire.isPending} data-testid="tracking-retire">
              {t("tracking.retire")}
            </Button>
          )}
        </div>
      ) : (
        canPublish && (
          <button type="button" onClick={() => m.publish.mutate(chosen)} disabled={!chosen || m.publish.isPending}
            className="rounded-md bg-blue-700 px-3 py-1.5 text-sm text-white disabled:opacity-60" data-testid="tracking-publish">
            {t("tracking.publish")}
          </button>
        )
      )}
    </section>
  );
}

function Row({ f, portalUrl, locale, canPublish, canRetire, onPublish, onRetire }: {
  f: DsarIntakeForm; portalUrl: string; locale: string; canPublish: boolean; canRetire: boolean;
  onPublish: () => void; onRetire: () => void;
}) {
  const t = useTranslations("dsarForms");
  const link = f.public_key ? `${portalUrl.replace(/\/$/, "")}/${locale}/dsar/${f.public_key}` : "";
  return (
    <tr className="border-t border-slate-100" data-testid={`form-row-${f.id}`}>
      <td className="py-1.5">{f.name}</td>
      <td>{t(`statuses.${f.status}`)}</td>
      <td className="break-all font-mono text-xs">{link && <a href={link} target="_blank" rel="noreferrer" className="text-blue-700 underline">{link}</a>}</td>
      <td className="space-x-2 text-right">
        {f.status !== "active" && canPublish && (
          <button type="button" onClick={onPublish} className="rounded-md border border-slate-300 px-2 py-1 text-xs" data-testid={`publish-${f.id}`}>
            {t("publish")}
          </button>
        )}
        {f.status !== "retired" && canRetire && (
          <button type="button" onClick={onRetire} className="rounded-md border border-slate-300 px-2 py-1 text-xs text-rose-700" data-testid={`retire-${f.id}`}>
            {t("retire")}
          </button>
        )}
      </td>
    </tr>
  );
}
