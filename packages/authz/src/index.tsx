"use client";

import { createContext, useContext, type ReactNode } from "react";

/** Mirrors the `Me` schema's roles/permissions/scopes (api/openapi/openapi.yaml). */
export interface Grants {
  roles: string[];
  permissions: string[];
  scopes: Array<{
    scope_type: "tenant" | "legal_entity" | "org_unit" | "self";
    legal_entity_id?: string | null;
    org_unit_id?: string | null;
    include_descendants?: boolean;
  }>;
}

const emptyGrants: Grants = { roles: [], permissions: [], scopes: [] };

const GrantsContext = createContext<Grants>(emptyGrants);

/** Wrap the authenticated app shell with the grants from GET /admin/v1/me. */
export function GrantsProvider({ grants, children }: { grants: Grants; children: ReactNode }) {
  return <GrantsContext.Provider value={grants}>{children}</GrantsContext.Provider>;
}

export function useGrants(): Grants {
  return useContext(GrantsContext);
}

/** code is an x-permission value, e.g. "ropa.activity.approve". */
export function usePermission(code: string): boolean {
  return useContext(GrantsContext).permissions.includes(code);
}

/**
 * Renders children only if the signed-in user has `permission`. Menu items and action buttons use
 * this instead of hiding by role, since a role's permission set can be tenant-customised
 * (docs/security/permissions.md).
 */
export function Can({ permission, children }: { permission: string; children: ReactNode }) {
  return usePermission(permission) ? <>{children}</> : null;
}
