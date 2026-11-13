// Package connectors (service.go) is PLT-20's own CRUD + test-connection layer over platform.connectors —
// the Driver/Registry SDK (driver.go, sql_driver.go, rest_driver.go) is the part a new connector_type
// extends; this file is the part that never changes when one does. Secrets are PLT-13-sealed via a
// Keyring (class "connector", context = the connector's own secret_ref string, so a forged/mismatched ref
// can never decrypt an unrelated connector's secret) in platform.connector_secrets, never on the
// platform.connectors row itself (rule 3 — never log or return a decrypted secret).
package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	pdb "pdpa-platform/internal/pkg/db"
	connectorsstore "pdpa-platform/internal/platform/connectors/store"
	"pdpa-platform/internal/platform/crypto"
)

var (
	ErrNotFound        = errors.New("connectors: not found")
	ErrInvalid         = errors.New("connectors: invalid input")
	ErrVersionMismatch = errors.New("connectors: version mismatch")
)

const secretClass = "connector"

// Ropa is the FK-visibility check for Config.AssetID (rule 1) — satisfied directly by the real
// ropaservice.Service in internal/wiring.Connectors(), the same cross-module pattern ROPA-02/DPO-01
// already established.
type Ropa interface {
	AssetVisible(ctx context.Context, id uuid.UUID) (bool, error)
}

// Connector is the wire-agnostic view of one platform.connectors row.
type Connector struct {
	ID            uuid.UUID
	ConnectorType string
	Name          string
	AssetID       *uuid.UUID
	Config        json.RawMessage
	HasSecret     bool
	Capabilities  []string
	Status        string
	LastTestedAt  *time.Time
	RowVersion    int32
	UpdatedAt     time.Time
}

// Input is what a caller may set; Secret is write-only — nil means "leave unchanged", a non-nil empty
// string clears it.
type Input struct {
	Name          string
	ConnectorType string
	AssetID       *uuid.UUID
	Config        json.RawMessage
	Secret        *string
	Capabilities  []string
}

// Filter narrows ListConnectors.
type Filter struct {
	ConnectorType string
	Status        string
	Cursor        *Cursor
	Limit         int
}

// Cursor is the position after the last connector of a page.
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

const pageSize = 50

// Service is PLT-20's own service: Registry decides which connector_types exist and what each can do;
// Service never switches on connector_type itself, which is the acceptance criterion's own proof.
type Service struct {
	Registry Registry
	Keyring  *crypto.Keyring
	Ropa     Ropa
}

func New(registry Registry, keyring *crypto.Keyring, ropa Ropa) *Service {
	return &Service{Registry: registry, Keyring: keyring, Ropa: ropa}
}

// ListTypes reads the registry live — never a hard-coded list — so a new Driver registered in
// internal/wiring.Connectors() shows up here with no change to this method.
func (s *Service) ListTypes() []Type {
	codes := make([]string, 0, len(s.Registry))
	for code := range s.Registry {
		codes = append(codes, code)
	}
	sortStrings(codes)
	out := make([]Type, 0, len(codes))
	for _, code := range codes {
		out = append(out, Type{Code: code, Capabilities: s.Registry[code].Capabilities})
	}
	return out
}

// Type is one registry entry's own public shape.
type Type struct {
	Code         string
	Capabilities []string
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j-1] > ss[j]; j-- {
			ss[j-1], ss[j] = ss[j], ss[j-1]
		}
	}
}

func (in *Input) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 200 {
		return fmt.Errorf("%w: name", ErrInvalid)
	}
	in.ConnectorType = strings.TrimSpace(in.ConnectorType)
	if in.ConnectorType == "" {
		return fmt.Errorf("%w: connector_type", ErrInvalid)
	}
	if len(in.Config) == 0 {
		in.Config = json.RawMessage(`{}`)
	}
	if !json.Valid(in.Config) {
		return fmt.Errorf("%w: config", ErrInvalid)
	}
	if in.Capabilities == nil {
		in.Capabilities = []string{}
	}
	return nil
}

// driverFor refuses an unknown connector_type and a capability not among the driver's own — the
// acceptance criterion's own "เพิ่ม connector ใหม่ได้โดยไม่แก้โมดูลหลัก" read the other way: a type this
// process's Registry doesn't carry is simply not creatable, with no special-cased error message per type.
func (s *Service) driverFor(connectorType string) (Driver, error) {
	d, ok := s.Registry[connectorType]
	if !ok {
		return Driver{}, fmt.Errorf("%w: unknown connector_type %q", ErrInvalid, connectorType)
	}
	return d, nil
}

func (s *Service) validateCapabilities(connectorType string, capabilities []string) error {
	d, err := s.driverFor(connectorType)
	if err != nil {
		return err
	}
	allowed := make(map[string]bool, len(d.Capabilities))
	for _, c := range d.Capabilities {
		allowed[c] = true
	}
	for _, c := range capabilities {
		if !allowed[c] {
			return fmt.Errorf("%w: capability %q not supported by connector_type %q", ErrInvalid, c, connectorType)
		}
	}
	return nil
}

func (s *Service) ListConnectors(ctx context.Context, f Filter) ([]Connector, *Cursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > pageSize {
		limit = pageSize
	}
	p := connectorsstore.ListConnectorsParams{Lim: int32(limit + 1)}
	if f.ConnectorType != "" {
		p.ConnectorType = &f.ConnectorType
	}
	if f.Status != "" {
		p.Status = &f.Status
	}
	if f.Cursor != nil {
		p.CursorAt = pgtype.Timestamptz{Time: f.Cursor.CreatedAt, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: f.Cursor.ID, Valid: true}
	}
	rows, err := connectorsstore.New(pdb.MustTxFromContext(ctx)).ListConnectors(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	var next *Cursor
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = &Cursor{CreatedAt: last.CreatedAt.Time, ID: last.ID}
	}
	out := make([]Connector, 0, len(rows))
	for _, r := range rows {
		out = append(out, Connector{
			ID:            r.ID,
			ConnectorType: r.ConnectorType,
			Name:          r.Name,
			AssetID:       uuidPtr(r.AssetID),
			Config:        json.RawMessage(r.Config),
			HasSecret:     false, // SecretRef is never selected in the list query (rule 3's own "never log")
			Capabilities:  r.Capabilities,
			Status:        r.Status,
			LastTestedAt:  tsPtr(r.LastTestedAt),
			RowVersion:    r.RowVersion,
			UpdatedAt:     r.UpdatedAt.Time,
		})
	}
	return out, next, nil
}

func (s *Service) GetConnector(ctx context.Context, id uuid.UUID) (Connector, error) {
	r, err := connectorsstore.New(pdb.MustTxFromContext(ctx)).GetConnector(ctx, id)
	if err != nil {
		return Connector{}, ErrNotFound
	}
	return Connector{
		ID:            r.ID,
		ConnectorType: r.ConnectorType,
		Name:          r.Name,
		AssetID:       uuidPtr(r.AssetID),
		Config:        json.RawMessage(r.Config),
		HasSecret:     r.SecretRef != nil && *r.SecretRef != "",
		Capabilities:  r.Capabilities,
		Status:        r.Status,
		LastTestedAt:  tsPtr(r.LastTestedAt),
		RowVersion:    r.RowVersion,
		UpdatedAt:     r.UpdatedAt.Time,
	}, nil
}

// CreateConnector always starts "draft" (the table's own column default) — a connector only becomes
// "active" by passing TestConnection, never by being saved.
func (s *Service) CreateConnector(ctx context.Context, in Input) (Connector, error) {
	if err := in.normalize(); err != nil {
		return Connector{}, err
	}
	if _, err := s.driverFor(in.ConnectorType); err != nil {
		return Connector{}, err
	}
	if err := s.validateCapabilities(in.ConnectorType, in.Capabilities); err != nil {
		return Connector{}, err
	}
	if in.AssetID != nil {
		if err := s.assetVisible(ctx, *in.AssetID); err != nil {
			return Connector{}, err
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Connector{}, err
	}
	var secretRef *string
	if in.Secret != nil && *in.Secret != "" {
		ref := id.String()
		if err := s.sealSecret(ctx, ref, *in.Secret); err != nil {
			return Connector{}, err
		}
		secretRef = &ref
	}

	r, err := connectorsstore.New(pdb.MustTxFromContext(ctx)).InsertConnector(ctx, connectorsstore.InsertConnectorParams{
		ID:            id,
		ConnectorType: in.ConnectorType,
		Name:          in.Name,
		AssetID:       uuidToPg(in.AssetID),
		Config:        in.Config,
		SecretRef:     secretRef,
		Capabilities:  in.Capabilities,
	})
	if err != nil {
		return Connector{}, err
	}
	return Connector{
		ID:            r.ID,
		ConnectorType: r.ConnectorType,
		Name:          r.Name,
		AssetID:       uuidPtr(r.AssetID),
		Config:        json.RawMessage(r.Config),
		HasSecret:     r.SecretRef != nil && *r.SecretRef != "",
		Capabilities:  r.Capabilities,
		Status:        r.Status,
		LastTestedAt:  tsPtr(r.LastTestedAt),
		RowVersion:    r.RowVersion,
		UpdatedAt:     r.UpdatedAt.Time,
	}, nil
}

// UpdateConnector never changes connector_type — the registry lookup a Driver call makes is resolved at
// call time from the stored type, so changing it after creation would silently swap which Driver a later
// Test/SearchSubject/Update/Delete call uses against config fields written for the old one.
func (s *Service) UpdateConnector(ctx context.Context, id uuid.UUID, in Input, version int32) (Connector, error) {
	cur, err := s.GetConnector(ctx, id)
	if err != nil {
		return Connector{}, err
	}
	in.ConnectorType = cur.ConnectorType
	if err := in.normalize(); err != nil {
		return Connector{}, err
	}
	if err := s.validateCapabilities(in.ConnectorType, in.Capabilities); err != nil {
		return Connector{}, err
	}
	if in.AssetID != nil {
		if err := s.assetVisible(ctx, *in.AssetID); err != nil {
			return Connector{}, err
		}
	}

	setSecret := in.Secret != nil
	var secretRef *string
	if setSecret {
		if *in.Secret == "" {
			if err := s.deleteSecret(ctx, id); err != nil {
				return Connector{}, err
			}
			secretRef = nil
		} else {
			ref := id.String()
			if err := s.sealSecret(ctx, ref, *in.Secret); err != nil {
				return Connector{}, err
			}
			secretRef = &ref
		}
	}

	r, err := connectorsstore.New(pdb.MustTxFromContext(ctx)).UpdateConnector(ctx, connectorsstore.UpdateConnectorParams{
		ID:           id,
		RowVersion:   version,
		Name:         in.Name,
		AssetID:      uuidToPg(in.AssetID),
		Config:       in.Config,
		Capabilities: in.Capabilities,
		SetSecretRef: setSecret,
		SecretRef:    secretRef,
	})
	if err != nil {
		return Connector{}, ErrVersionMismatch
	}
	return Connector{
		ID:            r.ID,
		ConnectorType: r.ConnectorType,
		Name:          r.Name,
		AssetID:       uuidPtr(r.AssetID),
		Config:        json.RawMessage(r.Config),
		HasSecret:     r.SecretRef != nil && *r.SecretRef != "",
		Capabilities:  r.Capabilities,
		Status:        r.Status,
		LastTestedAt:  tsPtr(r.LastTestedAt),
		RowVersion:    r.RowVersion,
		UpdatedAt:     r.UpdatedAt.Time,
	}, nil
}

func (s *Service) DeleteConnector(ctx context.Context, id uuid.UUID, version int32) error {
	if err := s.deleteSecret(ctx, id); err != nil {
		return err
	}
	n, err := connectorsstore.New(pdb.MustTxFromContext(ctx)).DeleteConnector(ctx, connectorsstore.DeleteConnectorParams{ID: id, RowVersion: version})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// TestConnection is the acceptance criterion's own literal half — "ทดสอบการเชื่อมต่อจากหน้าจอได้" — and
// never fails the HTTP call itself: a bad connection is reported through the status column, the same
// pattern cookie.domains' own verification flow already uses, so the admin screen shows a clear ok=false
// result rather than a generic 5xx.
func (s *Service) TestConnection(ctx context.Context, id uuid.UUID) (ok bool, message string, testedAt time.Time, _ Connector, _ error) {
	c, err := s.GetConnector(ctx, id)
	if err != nil {
		return false, "", time.Time{}, Connector{}, err
	}
	d, err := s.driverFor(c.ConnectorType)
	if err != nil {
		return false, "", time.Time{}, Connector{}, err
	}
	if d.Test == nil {
		return false, "", time.Time{}, Connector{}, fmt.Errorf("%w: test", ErrUnsupported)
	}
	cfg, err := s.config(ctx, c)
	if err != nil {
		return false, "", time.Time{}, Connector{}, err
	}

	testErr := d.Test(ctx, cfg)
	status := "active"
	msg := ""
	if testErr != nil {
		status = "error"
		msg = testErr.Error()
	}
	r, err := connectorsstore.New(pdb.MustTxFromContext(ctx)).SetConnectorTestResult(ctx, connectorsstore.SetConnectorTestResultParams{ID: id, Status: status})
	if err != nil {
		return false, "", time.Time{}, Connector{}, err
	}
	updated := Connector{
		ID:            r.ID,
		ConnectorType: r.ConnectorType,
		Name:          r.Name,
		AssetID:       uuidPtr(r.AssetID),
		Config:        json.RawMessage(r.Config),
		HasSecret:     r.SecretRef != nil && *r.SecretRef != "",
		Capabilities:  r.Capabilities,
		Status:        r.Status,
		LastTestedAt:  tsPtr(r.LastTestedAt),
		RowVersion:    r.RowVersion,
		UpdatedAt:     r.UpdatedAt.Time,
	}
	return testErr == nil, msg, r.LastTestedAt.Time, updated, nil
}

// SearchSubject runs one connector's own SearchSubject capability against a given identifier — DSAR-09's
// own cross-system search, PLT-20's module note naming it as a consumer from the start. Refuses a
// connector type whose Driver has no SearchSubject capability, or one whose own narrower Capabilities
// list doesn't offer it, with ErrUnsupported either way.
func (s *Service) SearchSubject(ctx context.Context, id uuid.UUID, identifier string) ([]SubjectRecord, error) {
	c, err := s.GetConnector(ctx, id)
	if err != nil {
		return nil, err
	}
	d, err := s.driverFor(c.ConnectorType)
	if err != nil {
		return nil, err
	}
	if d.SearchSubject == nil || !hasCapability(c.Capabilities, CapSearchSubject) {
		return nil, fmt.Errorf("%w: search_subject", ErrUnsupported)
	}
	cfg, err := s.config(ctx, c)
	if err != nil {
		return nil, err
	}
	return d.SearchSubject(ctx, cfg, identifier)
}

// Discover lists the tables/collections a connector can see — DFG-09's own first step, resolving which
// objects even exist before sampling any of them.
func (s *Service) Discover(ctx context.Context, id uuid.UUID) ([]DiscoveredTable, error) {
	c, err := s.GetConnector(ctx, id)
	if err != nil {
		return nil, err
	}
	d, err := s.driverFor(c.ConnectorType)
	if err != nil {
		return nil, err
	}
	if d.Discover == nil || !hasCapability(c.Capabilities, CapDiscover) {
		return nil, fmt.Errorf("%w: discover", ErrUnsupported)
	}
	cfg, err := s.config(ctx, c)
	if err != nil {
		return nil, err
	}
	return d.Discover(ctx, cfg)
}

// Sample pulls a small, bounded set of real rows from one table — DFG-09's own second step, run entirely
// in-process by the caller's classifiers; this method never persists a single value it returns.
func (s *Service) Sample(ctx context.Context, id uuid.UUID, table string, limit int) ([]SampledRow, error) {
	c, err := s.GetConnector(ctx, id)
	if err != nil {
		return nil, err
	}
	d, err := s.driverFor(c.ConnectorType)
	if err != nil {
		return nil, err
	}
	if d.Sample == nil || !hasCapability(c.Capabilities, CapSample) {
		return nil, fmt.Errorf("%w: sample", ErrUnsupported)
	}
	cfg, err := s.config(ctx, c)
	if err != nil {
		return nil, err
	}
	return d.Sample(ctx, cfg, table, limit)
}

// ListEmployees reads an HR connector's roster (DPX-13). Nothing it returns is persisted here.
func (s *Service) ListEmployees(ctx context.Context, id uuid.UUID) ([]Employee, error) {
	c, err := s.GetConnector(ctx, id)
	if err != nil {
		return nil, err
	}
	d, err := s.driverFor(c.ConnectorType)
	if err != nil {
		return nil, err
	}
	if d.ListEmployees == nil || !hasCapability(c.Capabilities, CapListEmployees) {
		return nil, fmt.Errorf("%w: list_employees", ErrUnsupported)
	}
	cfg, err := s.config(ctx, c)
	if err != nil {
		return nil, err
	}
	return d.ListEmployees(ctx, cfg)
}

// ListActiveWithCapability is the read side DSAR-09's own "ค้นข้อมูลจากระบบที่เชื่อมได้ในคำสั่งเดียว"
// needs: every one of this tenant's own active connectors that can actually be searched, across every
// page — small enough in practice (a handful of registered connectors per tenant) that paging through the
// whole list here, rather than exposing a capability filter on ListConnectors, is the simpler choice.
func (s *Service) ListActiveWithCapability(ctx context.Context, capability string) ([]Connector, error) {
	var out []Connector
	var cursor *Cursor
	for {
		page, next, err := s.ListConnectors(ctx, Filter{Status: "active", Cursor: cursor, Limit: pageSize})
		if err != nil {
			return nil, err
		}
		for _, c := range page {
			if hasCapability(c.Capabilities, capability) {
				out = append(out, c)
			}
		}
		if next == nil {
			break
		}
		cursor = next
	}
	return out, nil
}

func hasCapability(capabilities []string, capability string) bool {
	for _, c := range capabilities {
		if c == capability {
			return true
		}
	}
	return false
}

// config resolves a connector's own Driver Config — its decoded settings plus its decrypted secret, if
// any — the one place this package ever decrypts a secret (rule 3: never logged, never returned over the
// wire, used only in-process for the duration of one Driver call).
func (s *Service) config(ctx context.Context, c Connector) (Config, error) {
	var settings map[string]string
	if err := json.Unmarshal(c.Config, &settings); err != nil {
		// A non-string-map config (nested objects) is valid JSON but Driver.Config only carries flat
		// string settings — re-decode leniently into string values for any scalar.
		settings = flattenToStrings(c.Config)
	}
	cfg := Config{Settings: settings}
	if c.HasSecret {
		secret, err := s.readSecret(ctx, c.ID)
		if err != nil {
			return Config{}, err
		}
		cfg.Secret = secret
	}
	return cfg, nil
}

func flattenToStrings(raw json.RawMessage) map[string]string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if sv, ok := v.(string); ok {
			out[k] = sv
		} else {
			b, _ := json.Marshal(v)
			out[k] = string(b)
		}
	}
	return out
}

func (s *Service) assetVisible(ctx context.Context, id uuid.UUID) error {
	if s.Ropa == nil {
		return fmt.Errorf("%w: asset_id", ErrInvalid)
	}
	ok, err := s.Ropa.AssetVisible(ctx, id)
	if err != nil || !ok {
		return fmt.Errorf("%w: asset_id", ErrInvalid)
	}
	return nil
}

func (s *Service) sealSecret(ctx context.Context, ref, plaintext string) error {
	enc, err := s.Keyring.Encrypt(ctx, secretClass, ref, []byte(plaintext))
	if err != nil {
		return err
	}
	return connectorsstore.New(pdb.MustTxFromContext(ctx)).UpsertConnectorSecret(ctx, connectorsstore.UpsertConnectorSecretParams{
		ID:        mustUUIDv7(),
		SecretRef: ref,
		ValueEnc:  enc,
	})
}

func (s *Service) readSecret(ctx context.Context, id uuid.UUID) (string, error) {
	ref := id.String()
	enc, err := connectorsstore.New(pdb.MustTxFromContext(ctx)).GetConnectorSecret(ctx, ref)
	if err != nil {
		return "", nil // a vanished secret row is read as "no secret," never a hard failure of the call
	}
	plain, err := s.Keyring.Decrypt(ctx, secretClass, ref, enc)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *Service) deleteSecret(ctx context.Context, id uuid.UUID) error {
	return connectorsstore.New(pdb.MustTxFromContext(ctx)).DeleteConnectorSecret(ctx, id.String())
}

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	u := uuid.UUID(v.Bytes)
	return &u
}

func uuidToPg(v *uuid.UUID) pgtype.UUID {
	if v == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *v, Valid: true}
}

func tsPtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func mustUUIDv7() uuid.UUID {
	u, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return u
}
