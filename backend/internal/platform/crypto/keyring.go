package crypto

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	pdb "pdpa-platform/internal/pkg/db"
	cryptostore "pdpa-platform/internal/platform/crypto/store"
)

const (
	purposeDEK        = "dek"
	purposeBlindIndex = "blind_index"
	// blindIndexClass: one blind-index key per tenant (security.md); the identifier type goes into the
	// MAC input instead, so the same value under two types never collides.
	blindIndexClass = "default"

	formatV1 byte = 1
	cacheTTL      = 5 * time.Minute
)

// ErrDecrypt means a ciphertext could not be opened: wrong tenant, wrong data class or context,
// unknown key version, or tampered bytes.
var ErrDecrypt = errors.New("crypto: cannot decrypt")

// Keyring encrypts, decrypts and blind-indexes PII for the tenant of the transaction in ctx (opened
// by db.WithTenantTx). Data keys come from platform.tenant_keys (created on first use), unwrapped by
// KEK and cached in memory for cacheTTL. Safe for concurrent use; one per process.
type Keyring struct {
	KEK KEK
	Now func() time.Time // injectable for cache tests; default time.Now

	mu    sync.Mutex
	cache map[cacheKey]cached
}

type cacheKey struct {
	tenant  uuid.UUID
	purpose string
	class   string
	version int32
}

type cached struct {
	key     []byte
	expires time.Time
}

// Encrypt seals plaintext for a *_enc column with the tenant's active DEK for class. context names
// the column (e.g. "consent.subject_identifiers.value_enc") and is authenticated, so a ciphertext
// moved to another column, class or tenant fails to decrypt.
// Format: 0x01 | DEK version (uint32) | nonce | AES-256-GCM ciphertext+tag.
func (k *Keyring) Encrypt(ctx context.Context, class, context string, plaintext []byte) ([]byte, error) {
	tenantID, err := tenantOf(ctx)
	if err != nil {
		return nil, err
	}
	version, key, err := k.activeKey(ctx, tenantID, purposeDEK, class)
	if err != nil {
		return nil, err
	}
	sealed, err := sealGCM(key, plaintext, aad(tenantID, class, context))
	if err != nil {
		return nil, err
	}
	out := append([]byte{formatV1}, binary.BigEndian.AppendUint32(nil, uint32(version))...)
	return append(out, sealed...), nil
}

// Decrypt opens a value produced by Encrypt with the same class and context.
func (k *Keyring) Decrypt(ctx context.Context, class, context string, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 5 || ciphertext[0] != formatV1 {
		return nil, ErrDecrypt
	}
	tenantID, err := tenantOf(ctx)
	if err != nil {
		return nil, err
	}
	version := int32(binary.BigEndian.Uint32(ciphertext[1:5]))
	key, err := k.keyVersion(ctx, tenantID, purposeDEK, class, version)
	if err != nil {
		return nil, err
	}
	plain, err := openGCM(key, ciphertext[5:], aad(tenantID, class, context))
	if err != nil {
		return nil, ErrDecrypt
	}
	return plain, nil
}

// BlindIndex returns HMAC-SHA256(tenant blind-index key, kind | Normalize(kind, value)) for a
// blind_index column. Equal identifiers of one tenant give equal indexes; nothing about the value
// can be read back from it, and another tenant's index of the same value differs.
func (k *Keyring) BlindIndex(ctx context.Context, kind IdentifierKind, value string) ([]byte, error) {
	normalized, err := Normalize(kind, value)
	if err != nil {
		return nil, err
	}
	tenantID, err := tenantOf(ctx)
	if err != nil {
		return nil, err
	}
	_, key, err := k.activeKey(ctx, tenantID, purposeBlindIndex, blindIndexClass)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(kind))
	mac.Write([]byte{0})
	mac.Write([]byte(normalized))
	return mac.Sum(nil), nil
}

// RotateDEK makes a new DEK version the active one for class. New writes use it; values sealed with
// older versions keep decrypting (their versions are retired, not deleted).
func (k *Keyring) RotateDEK(ctx context.Context, class string) (int32, error) {
	tenantID, err := tenantOf(ctx)
	if err != nil {
		return 0, err
	}
	q := cryptostore.New(pdb.MustTxFromContext(ctx))
	prev, err := q.RetireActiveKey(ctx, cryptostore.RetireActiveKeyParams{Purpose: purposeDEK, DataClass: class})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("crypto: retire dek: %w", err)
	}
	version := prev + 1
	if err := k.createKey(ctx, q, tenantID, purposeDEK, class, version); err != nil {
		return 0, err
	}
	return version, nil
}

// RotateKEK rotates the tenant's key-encryption key and rewraps every data key of the tenant under the
// new version, in the transaction in ctx. No field data is re-encrypted.
func (k *Keyring) RotateKEK(ctx context.Context) (int, error) {
	tenantID, err := tenantOf(ctx)
	if err != nil {
		return 0, err
	}
	if err := k.KEK.Rotate(ctx, tenantID); err != nil {
		return 0, fmt.Errorf("crypto: rotate kek: %w", err)
	}
	q := cryptostore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListKeysForRewrap(ctx)
	if err != nil {
		return 0, fmt.Errorf("crypto: list keys: %w", err)
	}
	for _, row := range rows {
		wrapped, ref, err := k.KEK.Rewrap(ctx, tenantID, row.WrappedKey)
		if err != nil {
			return 0, fmt.Errorf("crypto: rewrap: %w", err)
		}
		if err := q.UpdateWrappedKey(ctx, cryptostore.UpdateWrappedKeyParams{ID: row.ID, WrappedKey: wrapped, KekRef: ref}); err != nil {
			return 0, fmt.Errorf("crypto: store rewrapped key: %w", err)
		}
	}
	return len(rows), nil
}

func (k *Keyring) activeKey(ctx context.Context, tenantID uuid.UUID, purpose, class string) (int32, []byte, error) {
	q := cryptostore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetActiveKey(ctx, cryptostore.GetActiveKeyParams{Purpose: purpose, DataClass: class})
	if errors.Is(err, pgx.ErrNoRows) {
		if err := k.createKey(ctx, q, tenantID, purpose, class, 1); err != nil {
			return 0, nil, err
		}
		row, err = q.GetActiveKey(ctx, cryptostore.GetActiveKeyParams{Purpose: purpose, DataClass: class})
	}
	if err != nil {
		return 0, nil, fmt.Errorf("crypto: read %s key: %w", purpose, err)
	}
	key, err := k.unwrapCached(ctx, cacheKey{tenantID, purpose, class, row.Version}, row.WrappedKey)
	return row.Version, key, err
}

func (k *Keyring) keyVersion(ctx context.Context, tenantID uuid.UUID, purpose, class string, version int32) ([]byte, error) {
	ck := cacheKey{tenantID, purpose, class, version}
	if key, ok := k.fromCache(ck); ok {
		return key, nil
	}
	row, err := cryptostore.New(pdb.MustTxFromContext(ctx)).GetKeyVersion(ctx, cryptostore.GetKeyVersionParams{Purpose: purpose, DataClass: class, Version: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDecrypt
	}
	if err != nil {
		return nil, fmt.Errorf("crypto: read %s key v%d: %w", purpose, version, err)
	}
	return k.unwrapCached(ctx, ck, row.WrappedKey)
}

// createKey generates a random 256-bit key, wraps it and stores it. Losing a concurrent race for the
// same (class, version) is fine: the insert does nothing and callers re-read the winner's key.
func (k *Keyring) createKey(ctx context.Context, q *cryptostore.Queries, tenantID uuid.UUID, purpose, class string, version int32) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	wrapped, ref, err := k.KEK.Wrap(ctx, tenantID, key)
	if err != nil {
		return fmt.Errorf("crypto: wrap %s key: %w", purpose, err)
	}
	if _, err := q.InsertKey(ctx, cryptostore.InsertKeyParams{
		Purpose: purpose, DataClass: class, Version: version, WrappedKey: wrapped, KekRef: ref,
	}); err != nil {
		return fmt.Errorf("crypto: store %s key: %w", purpose, err)
	}
	return nil
}

func (k *Keyring) unwrapCached(ctx context.Context, ck cacheKey, wrapped []byte) ([]byte, error) {
	if key, ok := k.fromCache(ck); ok {
		return key, nil
	}
	key, err := k.KEK.Unwrap(ctx, ck.tenant, wrapped)
	if err != nil {
		return nil, fmt.Errorf("crypto: unwrap %s key: %w", ck.purpose, err)
	}
	k.mu.Lock()
	if k.cache == nil {
		k.cache = map[cacheKey]cached{}
	}
	k.cache[ck] = cached{key: key, expires: k.now().Add(cacheTTL)}
	k.mu.Unlock()
	return key, nil
}

func (k *Keyring) fromCache(ck cacheKey) ([]byte, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	c, ok := k.cache[ck]
	if !ok {
		return nil, false
	}
	if k.now().After(c.expires) {
		delete(k.cache, ck)
		return nil, false
	}
	return c.key, true
}

func (k *Keyring) now() time.Time {
	if k.Now != nil {
		return k.Now()
	}
	return time.Now()
}

func tenantOf(ctx context.Context) (uuid.UUID, error) {
	var raw string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&raw); err != nil {
		return uuid.Nil, fmt.Errorf("crypto: tenant context: %w", err)
	}
	return uuid.Parse(raw)
}

func aad(tenantID uuid.UUID, class, context string) []byte {
	return []byte(tenantID.String() + "\x00" + class + "\x00" + context)
}
