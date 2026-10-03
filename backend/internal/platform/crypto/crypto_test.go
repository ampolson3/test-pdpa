package crypto_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/platform/crypto"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		kind       crypto.IdentifierKind
		in, want   string
		wantErrStr string
	}{
		{crypto.KindEmail, "  Somchai.J@Example.CO.TH ", "somchai.j@example.co.th", ""},
		{crypto.KindEmail, "not-an-email", "", "cannot be normalized"},
		{crypto.KindPhone, "081-234-5678", "+66812345678", ""},
		{crypto.KindPhone, "+66 81 234 5678", "+66812345678", ""},
		{crypto.KindPhone, "66812345678", "+66812345678", ""},
		{crypto.KindPhone, "02 123 4567", "+6621234567", ""},
		{crypto.KindPhone, "0066812345678", "+66812345678", ""},
		{crypto.KindPhone, "๐๘๑๒๓๔๕๖๗๘", "+66812345678", ""},
		{crypto.KindPhone, "call me", "", "cannot be normalized"},
		{crypto.KindNationalID, "1-2345-67890-12-3", "1234567890123", ""},
		{crypto.KindNationalID, "12345", "", "cannot be normalized"},
		{crypto.KindPassport, " aa 1234567 ", "AA1234567", ""},
		{crypto.KindCustomerID, " C-001 ", "C-001", ""},
		{"unknown", "x", "", "cannot be normalized"},
	}
	for _, tc := range cases {
		got, err := crypto.Normalize(tc.kind, tc.in)
		if tc.wantErrStr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErrStr) {
				t.Errorf("Normalize(%s, %q) err = %v, want %q", tc.kind, tc.in, err, tc.wantErrStr)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("Normalize(%s, %q) = %q, %v; want %q", tc.kind, tc.in, got, err, tc.want)
		}
	}
}

// keks returns the KEK implementations to run the suite against: always LocalKEK, plus OpenBao
// Transit when one is reachable at TEST_OPENBAO_ADDR (default the dev server at 127.0.0.1:8200).
func keks(t *testing.T) map[string]func() crypto.KEK {
	t.Helper()
	out := map[string]func() crypto.KEK{"local": func() crypto.KEK { return crypto.NewLocalKEK() }}
	addr := envOr("TEST_OPENBAO_ADDR", "http://127.0.0.1:8200")
	token := envOr("TEST_OPENBAO_TOKEN", "dev-root")
	res, err := (&http.Client{Timeout: time.Second}).Get(addr + "/v1/sys/health")
	if err == nil {
		res.Body.Close()
		out["transit"] = func() crypto.KEK { return &crypto.Transit{Addr: addr, Token: token} }
	} else {
		t.Logf("OpenBao not reachable at %s — running LocalKEK only (%v)", addr, err)
	}
	return out
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

type fixture struct {
	app  *pgxpool.Pool
	a, b dbtest.Tenant
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	app := dbtest.Pool(t)
	owner := dbtest.OwnerPool(t)
	platform := dbtest.PlatformPool(t)
	f := &fixture{app: app}
	f.a = dbtest.SeedTenant(t, ctx, app, platform, "crypto-a")
	f.b = dbtest.SeedTenant(t, ctx, app, platform, "crypto-b")
	// tenant_keys can't be deleted by the app (crypto-shredding is deliberate); the owner cleans up.
	t.Cleanup(func() {
		for _, tenant := range []dbtest.Tenant{f.a, f.b} {
			_ = pdb.WithTenantTx(context.Background(), app, tenant.ID.String(), "", func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				_, _ = tx.Exec(ctx, `DELETE FROM consent.subject_identifiers`)
				_, err := tx.Exec(ctx, `DELETE FROM consent.data_subjects`)
				return err
			})
			_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
				_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `DELETE FROM platform.tenant_keys`)
				return err
			})
		}
	})
	return f
}

func (f *fixture) in(t *testing.T, tenant dbtest.Tenant, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), "", fn); err != nil {
		t.Fatal(err)
	}
}

const (
	class   = "subject_identifier"
	column  = "consent.subject_identifiers.value_enc"
	email   = "somchai.j@example.co.th"
	emailUI = "  Somchai.J@Example.CO.TH "
)

// Acceptance criterion: "identifier ในฐานข้อมูลอ่านไม่ออกถ้าไม่มี key แต่ยังค้นหาแบบตรงตัวได้".
func TestKeyring_StoredIdentifierIsUnreadableButFindable(t *testing.T) {
	for name, newKEK := range keks(t) {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			kr := &crypto.Keyring{KEK: newKEK()}

			var subjectID uuid.UUID
			f.in(t, f.a, func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				if err := tx.QueryRow(ctx, `INSERT INTO consent.data_subjects (tenant_id, subject_key) VALUES ($1, 'S-1') RETURNING id`, f.a.ID).Scan(&subjectID); err != nil {
					return err
				}
				enc, err := kr.Encrypt(ctx, class, column, []byte(email))
				if err != nil {
					return err
				}
				bi, err := kr.BlindIndex(ctx, crypto.KindEmail, email)
				if err != nil {
					return err
				}
				_, err = tx.Exec(ctx, `INSERT INTO consent.subject_identifiers (tenant_id, subject_id, identifier_type, value_enc, blind_index)
					VALUES ($1, $2, 'email', $3, $4)`, f.a.ID, subjectID, enc, bi)
				return err
			})

			// What the database holds says nothing about the address.
			var raw, rawIndex []byte
			f.in(t, f.a, func(ctx context.Context) error {
				return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT value_enc, blind_index FROM consent.subject_identifiers WHERE subject_id = $1`, subjectID).Scan(&raw, &rawIndex)
			})
			for _, part := range []string{email, "somchai", "example"} {
				if bytes.Contains(bytes.ToLower(raw), []byte(part)) || bytes.Contains(rawIndex, []byte(part)) {
					t.Fatalf("stored bytes contain %q", part)
				}
			}

			// Exact lookup works from differently formatted input, and decrypts back.
			f.in(t, f.a, func(ctx context.Context) error {
				bi, err := kr.BlindIndex(ctx, crypto.KindEmail, emailUI)
				if err != nil {
					return err
				}
				var found []byte
				if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT value_enc FROM consent.subject_identifiers WHERE identifier_type = 'email' AND blind_index = $1`, bi).Scan(&found); err != nil {
					t.Errorf("lookup by blind index: %v", err)
					return nil
				}
				plain, err := kr.Decrypt(ctx, class, column, found)
				if err != nil || string(plain) != email {
					t.Errorf("decrypt = %q, %v; want %q", plain, err, email)
				}
				return nil
			})

			// Without the tenant's KEK the stored data key — and so the value — can't be opened.
			f.in(t, f.a, func(ctx context.Context) error {
				stranger := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
				if _, err := stranger.Decrypt(ctx, class, column, raw); err == nil {
					t.Error("decrypted without the tenant's KEK")
				}
				return nil
			})
		})
	}
}

// Tenant B can neither read tenant A's keys (RLS) nor decrypt or find tenant A's values.
func TestKeyring_TenantsAreIsolated(t *testing.T) {
	f := setup(t)
	kr := &crypto.Keyring{KEK: crypto.NewLocalKEK()}

	var ctA, biA []byte
	f.in(t, f.a, func(ctx context.Context) error {
		var err error
		if ctA, err = kr.Encrypt(ctx, class, column, []byte(email)); err != nil {
			return err
		}
		biA, err = kr.BlindIndex(ctx, crypto.KindEmail, email)
		return err
	})
	f.in(t, f.b, func(ctx context.Context) error {
		var n int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.tenant_keys WHERE tenant_id = $1`, f.a.ID).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("tenant B sees %d of tenant A's keys", n)
		}
		if _, err := kr.Decrypt(ctx, class, column, ctA); !errors.Is(err, crypto.ErrDecrypt) {
			t.Errorf("tenant B decrypting tenant A's value: %v, want ErrDecrypt", err)
		}
		biB, err := kr.BlindIndex(ctx, crypto.KindEmail, email)
		if err != nil {
			return err
		}
		if bytes.Equal(biA, biB) {
			t.Error("same e-mail has the same blind index in two tenants")
		}
		return nil
	})
}

// The context (column) and data class are authenticated: a ciphertext moved elsewhere won't open.
func TestKeyring_BindsCiphertextToItsColumn(t *testing.T) {
	f := setup(t)
	kr := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	f.in(t, f.a, func(ctx context.Context) error {
		ct, err := kr.Encrypt(ctx, class, column, []byte(email))
		if err != nil {
			return err
		}
		if _, err := kr.Decrypt(ctx, class, "dsar.requests.requester_contact_enc", ct); !errors.Is(err, crypto.ErrDecrypt) {
			t.Errorf("other column: %v, want ErrDecrypt", err)
		}
		if _, err := kr.Decrypt(ctx, "contact", column, ct); !errors.Is(err, crypto.ErrDecrypt) {
			t.Errorf("other class: %v, want ErrDecrypt", err)
		}
		ct[len(ct)-1] ^= 1
		if _, err := kr.Decrypt(ctx, class, column, ct); !errors.Is(err, crypto.ErrDecrypt) {
			t.Errorf("tampered bytes: %v, want ErrDecrypt", err)
		}
		return nil
	})
}

// Key rotation: a new DEK version serves new writes while old values still decrypt; rotating the KEK
// rewraps every data key without re-encrypting data.
func TestKeyring_Rotation(t *testing.T) {
	for name, newKEK := range keks(t) {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			kek := newKEK()
			kr := &crypto.Keyring{KEK: kek}

			var oldCT, newCT, oldBI []byte
			f.in(t, f.a, func(ctx context.Context) error {
				var err error
				if oldCT, err = kr.Encrypt(ctx, class, column, []byte("old@example.com")); err != nil {
					return err
				}
				if oldBI, err = kr.BlindIndex(ctx, crypto.KindEmail, "old@example.com"); err != nil {
					return err
				}
				v, err := kr.RotateDEK(ctx, class)
				if err != nil {
					return err
				}
				if v != 2 {
					t.Errorf("rotated DEK version = %d, want 2", v)
				}
				newCT, err = kr.Encrypt(ctx, class, column, []byte("new@example.com"))
				return err
			})
			if bytes.Equal(oldCT[1:5], newCT[1:5]) {
				t.Error("new value sealed with the old DEK version")
			}

			var refsBefore, refsAfter []string
			readRefs := func(dst *[]string) {
				f.in(t, f.a, func(ctx context.Context) error {
					rows, err := pdb.MustTxFromContext(ctx).Query(ctx, `SELECT kek_ref FROM platform.tenant_keys ORDER BY purpose, version`)
					if err != nil {
						return err
					}
					*dst, err = pgx.CollectRows(rows, pgx.RowTo[string])
					return err
				})
			}
			readRefs(&refsBefore)
			f.in(t, f.a, func(ctx context.Context) error {
				n, err := kr.RotateKEK(ctx)
				if err == nil && n != 3 {
					t.Errorf("rewrapped %d keys, want 3 (dek v1, dek v2, blind index)", n)
				}
				return err
			})
			readRefs(&refsAfter)
			for i := range refsBefore {
				if refsBefore[i] == refsAfter[i] {
					t.Errorf("key %d still wrapped by %s after KEK rotation", i, refsBefore[i])
				}
			}

			// A fresh keyring (empty cache) opens everything and indexes identically after both rotations.
			fresh := &crypto.Keyring{KEK: kek}
			f.in(t, f.a, func(ctx context.Context) error {
				for ct, want := range map[string]string{string(oldCT): "old@example.com", string(newCT): "new@example.com"} {
					got, err := fresh.Decrypt(ctx, class, column, []byte(ct))
					if err != nil || string(got) != want {
						t.Errorf("after rotation decrypt = %q, %v; want %q", got, err, want)
					}
				}
				bi, err := fresh.BlindIndex(ctx, crypto.KindEmail, "old@example.com")
				if err != nil || !bytes.Equal(bi, oldBI) {
					t.Errorf("blind index changed across KEK rotation (err %v)", err)
				}
				return nil
			})
		})
	}
}

// Concurrent first use of a data class creates exactly one key, and every value decrypts.
func TestKeyring_ConcurrentFirstUse(t *testing.T) {
	f := setup(t)
	kr := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	var wg sync.WaitGroup
	cts := make([][]byte, 10)
	errs := make([]error, 10)
	for i := range cts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
				var err error
				cts[i], err = kr.Encrypt(ctx, "contact", "dsar.requests.requester_contact_enc", []byte("0812345678"))
				return err
			})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}
	f.in(t, f.a, func(ctx context.Context) error {
		var n int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.tenant_keys WHERE data_class = 'contact'`).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("%d keys for class contact, want 1", n)
		}
		for i, ct := range cts {
			if _, err := kr.Decrypt(ctx, "contact", "dsar.requests.requester_contact_enc", ct); err != nil {
				t.Errorf("value %d: %v", i, err)
			}
		}
		return nil
	})
}

// Keys are never deleted by the application (crypto-shredding is a deliberate offboarding step).
func TestTenantKeys_AppCannotDelete(t *testing.T) {
	f := setup(t)
	kr := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	f.in(t, f.a, func(ctx context.Context) error {
		_, err := kr.BlindIndex(ctx, crypto.KindEmail, email)
		return err
	})
	err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `DELETE FROM platform.tenant_keys`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("DELETE as pdpa_app: %v, want permission denied", err)
	}
}
