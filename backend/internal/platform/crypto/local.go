package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// LocalKEK wraps keys with in-process AES-256-GCM master keys. For development and tests only: in
// every deployed environment the KEK lives in OpenBao Transit and never enters the application.
// Wrapped format: version (uint32) | nonce | ciphertext, with the tenant id as associated data so a
// wrapped key copied to another tenant does not unwrap.
type LocalKEK struct {
	mu       sync.Mutex
	versions [][]byte // index i = version i+1
}

// NewLocalKEK starts with one random master key version.
func NewLocalKEK() *LocalKEK {
	k := &LocalKEK{}
	k.addVersion()
	return k
}

func (k *LocalKEK) addVersion() {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	k.versions = append(k.versions, key)
}

func (k *LocalKEK) Wrap(_ context.Context, tenantID uuid.UUID, key []byte) ([]byte, string, error) {
	k.mu.Lock()
	version := len(k.versions)
	master := k.versions[version-1]
	k.mu.Unlock()
	sealed, err := sealGCM(master, key, tenantID[:])
	if err != nil {
		return nil, "", err
	}
	out := binary.BigEndian.AppendUint32(nil, uint32(version))
	return append(out, sealed...), fmt.Sprintf("local:v%d", version), nil
}

func (k *LocalKEK) Unwrap(_ context.Context, tenantID uuid.UUID, wrapped []byte) ([]byte, error) {
	if len(wrapped) < 4 {
		return nil, errors.New("crypto: local kek: wrapped key too short")
	}
	version := int(binary.BigEndian.Uint32(wrapped))
	k.mu.Lock()
	if version < 1 || version > len(k.versions) {
		k.mu.Unlock()
		return nil, fmt.Errorf("crypto: local kek: unknown version %d", version)
	}
	master := k.versions[version-1]
	k.mu.Unlock()
	return openGCM(master, wrapped[4:], tenantID[:])
}

func (k *LocalKEK) Rewrap(ctx context.Context, tenantID uuid.UUID, wrapped []byte) ([]byte, string, error) {
	key, err := k.Unwrap(ctx, tenantID, wrapped)
	if err != nil {
		return nil, "", err
	}
	return k.Wrap(ctx, tenantID, key)
}

// Rotate adds a master key version (shared by all tenants — this is a stand-in, not per tenant).
func (k *LocalKEK) Rotate(context.Context, uuid.UUID) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.addVersion()
	return nil
}

func sealGCM(key, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, aad), nil
}

func openGCM(key, sealed, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce, ct := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, aad)
}
