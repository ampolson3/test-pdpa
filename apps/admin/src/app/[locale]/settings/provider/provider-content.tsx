"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useTenants,
  useTenant,
  useCreateTenant,
  useSuspendTenant,
  useReactivateTenant,
  useTenantModules,
  useSetTenantModule,
  useCreateFirstAdmin,
  type ProviderTenant,
} from "@pdpa/api-client";

const statusStyle: Record<string, string> = {
  trial: "bg-slate-100 text-slate-700",
  active: "bg-green-100 text-green-800",
  suspended: "bg-amber-100 text-amber-800",
  terminated: "bg-red-100 text-red-800",
};

export function ProviderContent() {
  const t = useTranslations("provider");
  const allowed = usePermission("admin.tenant.read");

  if (!allowed) {
    return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;
  }
  return (
    <main className="mx-auto max-w-5xl space-y-6 p-8">
      <header>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-sm text-slate-600">{t("description")}</p>
      </header>
      <ProviderConsole />
    </main>
  );
}

function ProviderConsole() {
  const t = useTranslations("provider");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const canWrite = usePermission("admin.tenant.update");
  const canCreate = usePermission("admin.tenant.create");

  const [selected, setSelected] = useState<string | undefined>(undefined);
  const tenants = useTenants(client);
  const createTenant = useCreateTenant(client);
  const suspend = useSuspendTenant(client);
  const reactivate = useReactivateTenant(client);

  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [planCode, setPlanCode] = useState("");

  return (
    <>
      {canCreate && (
        <section className="rounded-md border border-slate-200 bg-white p-4">
          <h2 className="text-sm font-semibold text-slate-700">{t("create.title")}</h2>
          <form
            className="mt-3 flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              createTenant.mutate(
                { code, name, plan_code: planCode },
                { onSuccess: () => { setCode(""); setName(""); setPlanCode(""); } },
              );
            }}
          >
            <label className="text-sm">
              <span className="block text-slate-600">{t("create.code")}</span>
              <input className="mt-1 rounded-md border border-slate-300 px-2 py-1" value={code} onChange={(e) => setCode(e.target.value)} required maxLength={40} />
            </label>
            <label className="text-sm">
              <span className="block text-slate-600">{t("create.name")}</span>
              <input className="mt-1 rounded-md border border-slate-300 px-2 py-1" value={name} onChange={(e) => setName(e.target.value)} required />
            </label>
            <label className="text-sm">
              <span className="block text-slate-600">{t("create.planCode")}</span>
              <input className="mt-1 rounded-md border border-slate-300 px-2 py-1" value={planCode} onChange={(e) => setPlanCode(e.target.value)} required maxLength={40} />
            </label>
            <Button type="submit" disabled={createTenant.isPending}>{t("create.submit")}</Button>
          </form>
        </section>
      )}

      {tenants.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : tenants.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : !tenants.data || tenants.data.length === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : (
        <div className="overflow-x-auto rounded-md border border-slate-200 bg-white">
          <table className="w-full text-left text-sm">
            <thead className="bg-slate-50 text-slate-600">
              <tr>
                <th className="px-3 py-2 font-medium">{t("column.code")}</th>
                <th className="px-3 py-2 font-medium">{t("column.name")}</th>
                <th className="px-3 py-2 font-medium">{t("column.status")}</th>
                <th className="px-3 py-2 font-medium">{t("column.planCode")}</th>
                <th className="px-3 py-2 font-medium">{t("column.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {tenants.data.map((tenant) => (
                <TenantRow
                  key={tenant.id}
                  tenant={tenant}
                  selected={selected === tenant.id}
                  canWrite={!!canWrite}
                  onSelect={() => setSelected(selected === tenant.id ? undefined : tenant.id)}
                  onSuspend={() => suspend.mutate({ tenant })}
                  onReactivate={() => reactivate.mutate({ tenant })}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {selected && <TenantDetail client={client} tenantId={selected} canWrite={!!canWrite} />}
    </>
  );
}

function TenantRow({
  tenant, selected, canWrite, onSelect, onSuspend, onReactivate,
}: {
  tenant: ProviderTenant;
  selected: boolean;
  canWrite: boolean;
  onSelect: () => void;
  onSuspend: () => void;
  onReactivate: () => void;
}) {
  const t = useTranslations("provider");
  return (
    <tr className={`border-t border-slate-100 ${selected ? "bg-slate-50" : ""}`}>
      <td className="px-3 py-2 font-mono text-xs">{tenant.code}</td>
      <td className="px-3 py-2">{tenant.name}</td>
      <td className="px-3 py-2">
        <span className={`rounded-full px-2 py-0.5 text-xs ${statusStyle[tenant.status] ?? ""}`}>{t(`status.${tenant.status}`)}</span>
      </td>
      <td className="px-3 py-2">{tenant.plan_code}</td>
      <td className="px-3 py-2">
        <div className="flex gap-2">
          <Button variant="secondary" onClick={onSelect}>{t("manageModules")}</Button>
          {canWrite && (tenant.status === "suspended" ? (
            <Button variant="secondary" onClick={onReactivate}>{t("reactivate")}</Button>
          ) : (
            <Button variant="secondary" onClick={onSuspend}>{t("suspend")}</Button>
          ))}
        </div>
      </td>
    </tr>
  );
}

function TenantDetail({ client, tenantId, canWrite }: { client: ReturnType<typeof createApiClient>; tenantId: string; canWrite: boolean }) {
  const t = useTranslations("provider");
  const tenant = useTenant(client, tenantId);
  const modules = useTenantModules(client, tenantId);
  const setModule = useSetTenantModule(client, tenantId);
  const createAdmin = useCreateFirstAdmin(client, tenantId);

  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [roles, setRoles] = useState("ORGADMIN");

  return (
    <section className="space-y-4 rounded-md border border-slate-200 bg-white p-4">
      <h2 className="text-sm font-semibold text-slate-700">
        {t("modules.title", { name: tenant.data?.name ?? "" })}
      </h2>
      <p className="text-xs text-slate-500">{t("modules.toggle")}</p>
      {modules.data && (
        <ul className="divide-y divide-slate-100">
          {modules.data.map((m) => (
            <li key={m.module_code} className="flex items-center justify-between py-2 text-sm">
              <span className="font-mono text-xs">{m.module_code}</span>
              <label className="flex items-center gap-2">
                <span className="text-xs text-slate-600">{m.enabled ? t("modules.enabled") : t("modules.disabled")}</span>
                <input
                  type="checkbox"
                  checked={m.enabled}
                  disabled={!canWrite || setModule.isPending}
                  onChange={(e) => setModule.mutate({ module: m, input: { enabled: e.target.checked } })}
                />
              </label>
            </li>
          ))}
        </ul>
      )}

      {canWrite && (
        <div className="border-t border-slate-100 pt-4">
          <h3 className="text-sm font-semibold text-slate-700">{t("firstAdmin.title")}</h3>
          <p className="text-xs text-slate-500">{t("firstAdmin.note")}</p>
          <form
            className="mt-3 flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              createAdmin.mutate(
                { email, display_name: displayName, roles: roles.split(",").map((r) => r.trim()).filter(Boolean) },
                { onSuccess: () => { setEmail(""); setDisplayName(""); setRoles("ORGADMIN"); } },
              );
            }}
          >
            <label className="text-sm">
              <span className="block text-slate-600">{t("firstAdmin.email")}</span>
              <input type="email" className="mt-1 rounded-md border border-slate-300 px-2 py-1" value={email} onChange={(e) => setEmail(e.target.value)} required />
            </label>
            <label className="text-sm">
              <span className="block text-slate-600">{t("firstAdmin.displayName")}</span>
              <input className="mt-1 rounded-md border border-slate-300 px-2 py-1" value={displayName} onChange={(e) => setDisplayName(e.target.value)} required />
            </label>
            <label className="text-sm">
              <span className="block text-slate-600">{t("firstAdmin.roles")}</span>
              <input className="mt-1 rounded-md border border-slate-300 px-2 py-1" value={roles} onChange={(e) => setRoles(e.target.value)} required />
            </label>
            <Button type="submit" disabled={createAdmin.isPending}>{t("firstAdmin.submit")}</Button>
          </form>
        </div>
      )}
    </section>
  );
}
