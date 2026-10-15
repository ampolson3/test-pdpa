package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	cookiestore "pdpa-platform/internal/cookie/store"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/publickeys"
)

// DomainEntityType is this module's own audit entity type for cookie.domains.
const DomainEntityType = "cookie_domain"

// Domain is a website the banner SDK is embedded on.
type Domain struct {
	ID                uuid.UUID
	Domain            string
	LegalEntityID     uuid.UUID
	VerificationToken string
	Verified          bool
	Status            string // pending | active | disabled
	RowVersion        int32
	// PublicKey is the embed snippet's own data-domain-key — set only once the domain's banner has been
	// published at least once (PublishBannerConfig issues it, the same coalesce-on-first-publish pattern
	// DSAR-01/PNG-06/BRE-01 already use elsewhere).
	PublicKey *string
}

func toDomain(id uuid.UUID, domain, token string, legalEntity uuid.UUID, verifiedAt pgtype.Timestamptz, status string, rowVersion int32) Domain {
	return Domain{ID: id, Domain: domain, LegalEntityID: legalEntity, VerificationToken: token, Verified: verifiedAt.Valid, Status: status, RowVersion: rowVersion}
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RegisterDomain adds a website, pending verification. The legal entity must already be visible under the
// caller's own RLS (rule 1).
func (s *Service) RegisterDomain(ctx context.Context, domain string, legalEntityID uuid.UUID) (Domain, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.create") {
		return Domain{}, ErrForbidden
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" || len(domain) > 255 {
		return Domain{}, fmt.Errorf("%w: domain", ErrInvalid)
	}
	if _, err := s.Org.GetLegalEntity(ctx, legalEntityID); err != nil {
		return Domain{}, fmt.Errorf("%w: legal_entity_id", ErrInvalid)
	}
	token, err := randomToken()
	if err != nil {
		return Domain{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Domain{}, err
	}
	var row cookiestore.InsertDomainRow
	if err := pdb.Savepoint(ctx, func(ctx context.Context) error {
		var err error
		row, err = cookiestore.New(pdb.MustTxFromContext(ctx)).InsertDomain(ctx, cookiestore.InsertDomainParams{
			ID: id, Domain: domain, LegalEntityID: legalEntityID, VerificationToken: token,
		})
		return err
	}); err != nil {
		return Domain{}, fmt.Errorf("%w: domain already registered", ErrInvalid)
	}
	d := toDomain(row.ID, row.Domain, row.VerificationToken, row.LegalEntityID, row.VerifiedAt, row.Status, row.RowVersion)
	if err := s.audit(ctx, "cookie.domain.register", DomainEntityType, id, nil, map[string]any{"domain": domain}); err != nil {
		return Domain{}, err
	}
	// CON-02's own backend note calls for 4 default categories (Strictly Necessary / Functional /
	// Analytics / Marketing) to exist out of the box — a tunable convenience, not legal content (rule 8
	// doesn't apply: these are internal category labels, not notice/consent wording shown to a data
	// subject), so no decisions.md entry is needed. Seeded once per tenant, on its first domain, rather
	// than via a migration: cookie.categories has no tenant_id-NULL global-row pattern like ORG-07's
	// master data (tenant_id is NOT NULL here), so a migration can't seed rows visible to every tenant.
	if err := s.seedDefaultCategoriesIfNone(ctx); err != nil {
		return Domain{}, err
	}
	return d, nil
}

var defaultCategories = []CreateCategoryInput{
	{Code: "necessary", NameTh: "จำเป็นอย่างยิ่ง", NameEn: "Strictly Necessary", IsRequired: true, DisplayOrder: 0},
	{Code: "functional", NameTh: "การทำงาน", NameEn: "Functional", DisplayOrder: 1},
	{Code: "analytics", NameTh: "วิเคราะห์", NameEn: "Analytics", DisplayOrder: 2},
	{Code: "marketing", NameTh: "การตลาด", NameEn: "Marketing", DisplayOrder: 3},
}

func (s *Service) seedDefaultCategoriesIfNone(ctx context.Context) error {
	existing, err := cookiestore.New(pdb.MustTxFromContext(ctx)).ListCategories(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	for _, in := range defaultCategories {
		if _, err := s.CreateCategory(ctx, in); err != nil {
			return err
		}
	}
	return nil
}

// withPublicKey fills in the install snippet's own key, when one has been issued (a domain whose banner
// was never published has none yet).
func withPublicKey(ctx context.Context, d Domain) (Domain, error) {
	key, ok, err := publickeys.Get(ctx, publickeys.EntityCookieDomain, d.ID)
	if err != nil {
		return Domain{}, err
	}
	if ok {
		d.PublicKey = &key
	}
	return d, nil
}

// GetDomain returns one domain the caller's tenant owns.
func (s *Service) GetDomain(ctx context.Context, id uuid.UUID) (Domain, error) {
	row, err := cookiestore.New(pdb.MustTxFromContext(ctx)).GetDomain(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Domain{}, ErrNotFound
	}
	if err != nil {
		return Domain{}, err
	}
	return withPublicKey(ctx, toDomain(row.ID, row.Domain, row.VerificationToken, row.LegalEntityID, row.VerifiedAt, row.Status, row.RowVersion))
}

// ListDomains returns every domain registered for the tenant, newest first.
func (s *Service) ListDomains(ctx context.Context) ([]Domain, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.read") {
		return nil, ErrForbidden
	}
	rows, err := cookiestore.New(pdb.MustTxFromContext(ctx)).ListDomains(ctx, cookiestore.ListDomainsParams{Lim: 200})
	if err != nil {
		return nil, err
	}
	out := make([]Domain, 0, len(rows))
	for _, r := range rows {
		d, err := withPublicKey(ctx, toDomain(r.ID, r.Domain, r.VerificationToken, r.LegalEntityID, r.VerifiedAt, r.Status, r.RowVersion))
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// VerifyDomain is this pass's own manual confirmation that the caller controls the domain — no automated
// DNS TXT / meta-tag check exists in this environment (no outbound DNS resolution available to verify
// against); the token is still issued and shown so a future pass can add that check without touching the
// data model. Marks the domain active.
func (s *Service) VerifyDomain(ctx context.Context, id uuid.UUID, rowVersion int32) (Domain, error) {
	g, _ := authz.FromContext(ctx)
	if !g.Has("cookie.banner.update") {
		return Domain{}, ErrForbidden
	}
	if _, err := s.GetDomain(ctx, id); err != nil {
		return Domain{}, err
	}
	n, err := cookiestore.New(pdb.MustTxFromContext(ctx)).VerifyDomain(ctx, cookiestore.VerifyDomainParams{ID: id, RowVersion: rowVersion})
	if err != nil {
		return Domain{}, err
	}
	if n == 0 {
		return Domain{}, ErrVersionMismatch
	}
	if err := s.audit(ctx, "cookie.domain.verify", DomainEntityType, id, nil, nil); err != nil {
		return Domain{}, err
	}
	return s.GetDomain(ctx, id)
}
