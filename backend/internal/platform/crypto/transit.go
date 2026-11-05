package crypto

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Transit is the KEK backed by OpenBao's Transit secrets engine: one aes256-gcm96 key per tenant,
// named "tenant-<uuid>", created on first use. The token comes from the environment or OpenBao agent
// (CLAUDE.md rule 14) and needs create/update on <mount>/keys/tenant-*, <mount>/encrypt/tenant-*,
// <mount>/decrypt/tenant-*, <mount>/rewrap/tenant-* and <mount>/keys/tenant-*/rotate.
type Transit struct {
	Addr  string // e.g. https://openbao.internal:8200
	Token string
	Mount string // default "transit"
	HTTP  *http.Client

	mu      sync.Mutex
	ensured map[uuid.UUID]bool
}

func (t *Transit) keyName(tenantID uuid.UUID) string { return "tenant-" + tenantID.String() }

func (t *Transit) Wrap(ctx context.Context, tenantID uuid.UUID, key []byte) ([]byte, string, error) {
	if err := t.ensureKey(ctx, tenantID); err != nil {
		return nil, "", err
	}
	var out struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	if err := t.call(ctx, "encrypt/"+t.keyName(tenantID), map[string]any{"plaintext": base64.StdEncoding.EncodeToString(key)}, &out); err != nil {
		return nil, "", err
	}
	return []byte(out.Data.Ciphertext), t.ref(tenantID, out.Data.Ciphertext), nil
}

func (t *Transit) Unwrap(ctx context.Context, tenantID uuid.UUID, wrapped []byte) ([]byte, error) {
	var out struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := t.call(ctx, "decrypt/"+t.keyName(tenantID), map[string]any{"ciphertext": string(wrapped)}, &out); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Data.Plaintext)
}

func (t *Transit) Rewrap(ctx context.Context, tenantID uuid.UUID, wrapped []byte) ([]byte, string, error) {
	var out struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	if err := t.call(ctx, "rewrap/"+t.keyName(tenantID), map[string]any{"ciphertext": string(wrapped)}, &out); err != nil {
		return nil, "", err
	}
	return []byte(out.Data.Ciphertext), t.ref(tenantID, out.Data.Ciphertext), nil
}

func (t *Transit) Rotate(ctx context.Context, tenantID uuid.UUID) error {
	if err := t.ensureKey(ctx, tenantID); err != nil {
		return err
	}
	return t.call(ctx, "keys/"+t.keyName(tenantID)+"/rotate", map[string]any{}, nil)
}

// ref is "transit:<key name>:<version>", version taken from the ciphertext prefix ("vault:v3:…").
func (t *Transit) ref(tenantID uuid.UUID, ciphertext string) string {
	parts := strings.SplitN(ciphertext, ":", 3)
	version := "v?"
	if len(parts) == 3 {
		version = parts[1]
	}
	return "transit:" + t.keyName(tenantID) + ":" + version
}

// ensureKey creates the tenant's key once per process (creating an existing key is a no-op).
func (t *Transit) ensureKey(ctx context.Context, tenantID uuid.UUID) error {
	t.mu.Lock()
	done := t.ensured[tenantID]
	t.mu.Unlock()
	if done {
		return nil
	}
	if err := t.call(ctx, "keys/"+t.keyName(tenantID), map[string]any{"type": "aes256-gcm96"}, nil); err != nil {
		return err
	}
	t.mu.Lock()
	if t.ensured == nil {
		t.ensured = map[uuid.UUID]bool{}
	}
	t.ensured[tenantID] = true
	t.mu.Unlock()
	return nil
}

func (t *Transit) call(ctx context.Context, path string, body any, out any) error {
	mount := t.Mount
	if mount == "" {
		mount = "transit"
	}
	client := t.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(t.Addr, "/")+"/v1/"+mount+"/"+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", t.Token)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("crypto: openbao %s: %w", mount, err)
	}
	defer res.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode/100 != 2 {
		// The response body names the failure (e.g. permission denied); it never echoes key material.
		return fmt.Errorf("crypto: openbao %s/%s: status %d: %s", mount, strings.SplitN(path, "/", 2)[0], res.StatusCode, strings.TrimSpace(string(payload)))
	}
	if out == nil || len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, out)
}
