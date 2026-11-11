package wiring

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"pdpa-platform/internal/platform/connectors"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// Connectors builds the one Registry cmd/api and cmd/worker both share — the acceptance criterion's own
// "เพิ่ม connector ใหม่ได้โดยไม่แก้โมดูลหลัก" holds because adding a type is exactly one new line here
// (or a brand-new Driver value elsewhere), never a change to internal/platform/connectors's own Service.
func Connectors() connectors.Registry {
	return connectors.Registry{
		"postgres":  connectors.PostgresDriver(),
		"mysql":     connectors.MySQLDriver(),
		"sqlserver": connectors.SQLServerDriver(),
		"rest":      connectors.RESTDriver(),
	}
}

// ConnectorsRopa adapts the real ropa service to connectors's own local Ropa interface (PLT-20's
// FK-visibility check on Config.AssetID, rule 1) — kept as a plain bool rather than importing ropa's own
// Asset type, so internal/platform/connectors never depends on a business module at all (rule 9).
type ConnectorsRopa struct {
	Ropa *ropaservice.Service
}

func (a ConnectorsRopa) AssetVisible(ctx context.Context, id uuid.UUID) (bool, error) {
	_, err := a.Ropa.GetAsset(ctx, id)
	if err != nil {
		if errors.Is(err, ropaservice.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
