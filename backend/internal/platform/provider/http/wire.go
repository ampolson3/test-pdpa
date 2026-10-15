package providerhttp

import (
	"encoding/json"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	provider "pdpa-platform/internal/platform/provider"
)

func toTenantWire(t provider.Tenant) ProviderTenant {
	w := ProviderTenant{
		Id: t.ID, Code: t.Code, Name: t.Name, Status: ProviderTenantStatus(t.Status), PlanCode: t.PlanCode,
		Deployment: ProviderTenantDeployment(t.Deployment), DataRegion: t.DataRegion,
		RowVersion: int(t.RowVersion), CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
	}
	if t.SuspendedAt != nil {
		u := t.SuspendedAt.UTC()
		w.SuspendedAt = &u
	}
	return w
}

func toHealthWire(h provider.Health) ProviderHealth {
	return ProviderHealth{
		Service:       h.Service,
		WindowMinutes: h.WindowMinutes,
		RequestCount:  h.RequestCount,
		ErrorCount:    h.ErrorCount,
		ErrorRate:     float32(h.ErrorRate),
		AvgLatencyMs:  float32(h.AvgLatencyMs),
		SlowCount:     h.SlowCount,
		SlowRatio:     float32(h.SlowRatio),
		Status:        ProviderHealthStatus(h.Status),
		CheckedAt:     h.CheckedAt.UTC(),
	}
}

func toModuleWire(m provider.TenantModule) ProviderTenantModule {
	w := ProviderTenantModule{TenantId: m.TenantID, ModuleCode: m.ModuleCode, Enabled: m.Enabled, RowVersion: int(m.RowVersion)}
	if m.LicenseExpiresAt != nil {
		d := openapi_types.Date{Time: *m.LicenseExpiresAt}
		w.LicenseExpiresAt = &d
	}
	var q map[string]any
	if len(m.Quota) > 0 {
		_ = json.Unmarshal(m.Quota, &q)
	}
	if q == nil {
		q = map[string]any{}
	}
	w.Quota = &q
	return w
}

func dateTimePtr(d *openapi_types.Date) *time.Time {
	if d == nil {
		return nil
	}
	t := d.Time
	return &t
}

func quotaJSON(q map[string]any) json.RawMessage {
	if q == nil {
		return nil
	}
	b, _ := json.Marshal(q)
	return b
}
