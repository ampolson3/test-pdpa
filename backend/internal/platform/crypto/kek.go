// Package crypto is field-level PII encryption for the platform (PLT-13, docs/architecture/security.md
// § ลำดับชั้นกุญแจ):
//
//   - KEK: one key-encryption key per tenant in OpenBao Transit (Transit); it never leaves OpenBao.
//     LocalKEK stands in for development and tests only.
//   - DEK: one AES-256-GCM data key per tenant and data class, stored in platform.tenant_keys wrapped by
//     the tenant's KEK, unwrapped on demand and cached in memory for a few minutes (Keyring).
//   - *_enc columns: Keyring.Encrypt / Decrypt (AES-256-GCM, versioned so DEKs can rotate).
//   - blind_index columns: Keyring.BlindIndex — HMAC-SHA256 with a per-tenant key over the normalized
//     value, for exact-match lookup without decrypting.
//
// Rotation: RotateKEK rotates the tenant's Transit key and rewraps its DEKs (no data is re-encrypted);
// RotateDEK starts a new DEK version for new writes while old versions keep decrypting old data.
package crypto

import (
	"context"

	"github.com/google/uuid"
)

// KEK wraps and unwraps data keys with a tenant's key-encryption key. ref identifies the KEK version
// that produced wrapped, and is stored next to it (platform.tenant_keys.kek_ref).
type KEK interface {
	Wrap(ctx context.Context, tenantID uuid.UUID, key []byte) (wrapped []byte, ref string, err error)
	Unwrap(ctx context.Context, tenantID uuid.UUID, wrapped []byte) ([]byte, error)
	// Rewrap re-encrypts wrapped under the tenant's latest KEK version without exposing the key.
	Rewrap(ctx context.Context, tenantID uuid.UUID, wrapped []byte) (newWrapped []byte, ref string, err error)
	// Rotate creates a new KEK version for the tenant; existing wrapped keys stay unwrappable.
	Rotate(ctx context.Context, tenantID uuid.UUID) error
}
