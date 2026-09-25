package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	consentstore "pdpa-platform/internal/consent/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/publickeys"
)

// Channels a collection point may use here (import and cookie points are made by their own features).
var channels = []string{"web", "app", "pos", "call_center", "kiosk", "paper", "line", "api"}

// Checklist is the s.19 attestation made when publishing: the consent request is separate from other terms, isn't
// a condition of a contract beyond what is needed, is in plain language, and says how to withdraw. The form itself
// never pre-ticks a purpose (the renderer and the recorder both enforce it).
type Checklist struct {
	SeparateText   bool `json:"separate_text"`
	NotBundled     bool `json:"not_bundled"`
	PlainLanguage  bool `json:"plain_language"`
	WithdrawalInfo bool `json:"withdrawal_info"`
}

// CPPurpose is a purpose shown by a collection point.
type CPPurpose struct {
	PurposeID        uuid.UUID
	Code             string
	Name             Text
	Status           string
	IsSensitive      bool
	Required         bool
	CurrentVersionID *uuid.UUID
	CurrentVersionNo *int32
	ConsentText      Text
	ExplicitText     Text
	MinAge           *int16
	LifespanDays     *int32
}

// CollectionPoint is a place consent is collected (web form, app, branch counter, call centre, API).
type CollectionPoint struct {
	ID             uuid.UUID
	Code           string
	Name           string
	Channel        string
	LegalEntityID  uuid.UUID
	Status         string // draft | active | retired
	PublicKey      string // issued on the first publish
	AllowedOrigins []string
	Checklist      *Checklist
	PublishedAt    *time.Time
	Purposes       []CPPurpose
	RowVersion     int32
	UpdatedAt      time.Time
}

// CollectionPointInput is what the editor saves.
type CollectionPointInput struct {
	Code           string
	Name           string
	Channel        string
	LegalEntityID  uuid.UUID
	AllowedOrigins []string
	Purposes       []CPPurposeInput
}

// CPPurposeInput places a purpose on the form; Required purposes must be consented to to go on (never a sensitive one).
type CPPurposeInput struct {
	PurposeID uuid.UUID
	Required  bool
}

func (s *Service) checkCPInput(ctx context.Context, in *CollectionPointInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 200 {
		return invalid("name")
	}
	if !slices.Contains(channels, in.Channel) {
		return invalid("channel")
	}
	if _, err := s.Org.GetLegalEntity(ctx, in.LegalEntityID); err != nil {
		return invalid("legal entity")
	}
	if in.AllowedOrigins == nil {
		in.AllowedOrigins = []string{}
	}
	for i, o := range in.AllowedOrigins {
		u, err := url.Parse(strings.TrimSpace(o))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
			return invalid("origin %q", o)
		}
		in.AllowedOrigins[i] = u.Scheme + "://" + u.Host
	}
	if len(in.Purposes) > 50 {
		return invalid("too many purposes")
	}
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	seen := map[uuid.UUID]bool{}
	for _, p := range in.Purposes {
		if seen[p.PurposeID] {
			return invalid("purpose listed twice")
		}
		seen[p.PurposeID] = true
		if _, err := q.GetPurpose(ctx, p.PurposeID); err != nil { // visible under RLS
			return invalid("purpose")
		}
	}
	return nil
}

// CreateCollectionPoint adds a draft collection point.
func (s *Service) CreateCollectionPoint(ctx context.Context, in CollectionPointInput) (CollectionPoint, error) {
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if !codeRE.MatchString(in.Code) {
		return CollectionPoint{}, invalid("code")
	}
	if err := s.checkCPInput(ctx, &in); err != nil {
		return CollectionPoint{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return CollectionPoint{}, err
	}
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		return consentstore.New(pdb.MustTxFromContext(ctx)).InsertCollectionPoint(ctx, consentstore.InsertCollectionPointParams{ID: id, Code: in.Code, Name: in.Name,
			Channel: in.Channel, LegalEntityID: in.LegalEntityID, AllowedOrigins: in.AllowedOrigins})
	})
	if isUnique(err) {
		return CollectionPoint{}, invalid("code exists")
	}
	if err != nil {
		return CollectionPoint{}, err
	}
	if err := s.setCPPurposes(ctx, id, in.Purposes); err != nil {
		return CollectionPoint{}, err
	}
	if err := s.audit(ctx, "consent.collection_point.create", CollectionPointType, id, nil, map[string]any{"code": in.Code, "channel": in.Channel}); err != nil {
		return CollectionPoint{}, err
	}
	return s.GetCollectionPoint(ctx, id)
}

// UpdateCollectionPoint saves the editor (If-Match). An active point keeps its key; its purposes must still pass
// the publish checks, since the form is live.
func (s *Service) UpdateCollectionPoint(ctx context.Context, id uuid.UUID, version int32, in CollectionPointInput) (CollectionPoint, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.LockCollectionPoint(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return CollectionPoint{}, ErrNotFound
	}
	if err != nil {
		return CollectionPoint{}, err
	}
	if cur.RowVersion != version {
		return CollectionPoint{}, ErrVersionMismatch
	}
	if cur.Status == "retired" {
		return CollectionPoint{}, ErrInvalidTransition
	}
	if err := s.checkCPInput(ctx, &in); err != nil {
		return CollectionPoint{}, err
	}
	if n, err := q.UpdateCollectionPoint(ctx, consentstore.UpdateCollectionPointParams{ID: id, RowVersion: version, Name: in.Name, Channel: in.Channel,
		LegalEntityID: in.LegalEntityID, AllowedOrigins: in.AllowedOrigins}); err != nil || n == 0 {
		if err == nil {
			err = ErrVersionMismatch
		}
		return CollectionPoint{}, err
	}
	if err := s.setCPPurposes(ctx, id, in.Purposes); err != nil {
		return CollectionPoint{}, err
	}
	if cur.Status == "active" {
		cp, err := s.GetCollectionPoint(ctx, id)
		if err != nil {
			return CollectionPoint{}, err
		}
		if failed := cpChecks(cp, Checklist{true, true, true, true}); len(failed) > 0 {
			return CollectionPoint{}, &CheckError{Failed: failed}
		}
		if cur.PublicKey != nil {
			if err := publickeys.SetOrigins(ctx, *cur.PublicKey, in.AllowedOrigins); err != nil {
				return CollectionPoint{}, err
			}
		}
	}
	if err := s.audit(ctx, "consent.collection_point.update", CollectionPointType, id, nil, map[string]any{"name": in.Name, "purposes": len(in.Purposes)}); err != nil {
		return CollectionPoint{}, err
	}
	return s.GetCollectionPoint(ctx, id)
}

func (s *Service) setCPPurposes(ctx context.Context, id uuid.UUID, ps []CPPurposeInput) error {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	if err := q.DeleteCollectionPointPurposes(ctx, id); err != nil {
		return err
	}
	for i, p := range ps {
		if err := q.InsertCollectionPointPurpose(ctx, consentstore.InsertCollectionPointPurposeParams{CollectionPointID: id, PurposeID: p.PurposeID, IsRequired: p.Required, DisplayOrder: int16(i)}); err != nil {
			return err
		}
	}
	return nil
}

// cpChecks are the publish rules (s.19 / CON-09, s.26 / CON-10). Codes are localized by the UI.
func cpChecks(cp CollectionPoint, c Checklist) []string {
	var failed []string
	if len(cp.Purposes) == 0 {
		failed = append(failed, "no_purposes")
	}
	for _, p := range cp.Purposes {
		if p.Status != "active" || p.CurrentVersionID == nil {
			failed = append(failed, "purpose_not_published")
			break
		}
	}
	for _, p := range cp.Purposes {
		if p.IsSensitive && p.Required {
			failed = append(failed, "sensitive_required") // explicit consent can't be a condition of the service
			break
		}
	}
	for _, p := range cp.Purposes {
		if p.IsSensitive && p.ExplicitText.Th == "" {
			failed = append(failed, "sensitive_without_explicit_text")
			break
		}
	}
	if !c.SeparateText {
		failed = append(failed, "checklist_separate_text")
	}
	if !c.NotBundled {
		failed = append(failed, "checklist_not_bundled")
	}
	if !c.PlainLanguage {
		failed = append(failed, "checklist_plain_language")
	}
	if !c.WithdrawalInfo {
		failed = append(failed, "checklist_withdrawal_info")
	}
	return failed
}

// PublishCollectionPoint makes a draft (or re-publishes an active) point live after the checks: it gets a public
// key (platform.public_keys) that forms, links, QR codes and SDKs use.
func (s *Service) PublishCollectionPoint(ctx context.Context, id uuid.UUID, version int32, c Checklist) (CollectionPoint, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.LockCollectionPoint(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return CollectionPoint{}, ErrNotFound
	}
	if err != nil {
		return CollectionPoint{}, err
	}
	if cur.RowVersion != version {
		return CollectionPoint{}, ErrVersionMismatch
	}
	if cur.Status == "retired" {
		return CollectionPoint{}, ErrInvalidTransition
	}
	cp, err := s.GetCollectionPoint(ctx, id)
	if err != nil {
		return CollectionPoint{}, err
	}
	if failed := cpChecks(cp, c); len(failed) > 0 {
		return CollectionPoint{}, &CheckError{Failed: failed}
	}
	key := ""
	if cur.PublicKey == nil {
		if key, err = publickeys.Issue(ctx, publickeys.EntityCollectionPoint, id, cp.AllowedOrigins); err != nil {
			return CollectionPoint{}, err
		}
	}
	checklist, _ := json.Marshal(c)
	if err := q.PublishCollectionPoint(ctx, consentstore.PublishCollectionPointParams{ID: id, PublicKey: optText(key), Checklist: checklist}); err != nil {
		return CollectionPoint{}, err
	}
	if err := s.audit(ctx, "consent.collection_point.publish", CollectionPointType, id, map[string]any{"status": cur.Status}, map[string]any{"status": "active", "checklist": c}); err != nil {
		return CollectionPoint{}, err
	}
	return s.GetCollectionPoint(ctx, id)
}

// RetireCollectionPoint closes a point for good: its key stops resolving; its receipts stay.
func (s *Service) RetireCollectionPoint(ctx context.Context, id uuid.UUID, version int32) (CollectionPoint, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.LockCollectionPoint(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return CollectionPoint{}, ErrNotFound
	}
	if err != nil {
		return CollectionPoint{}, err
	}
	if cur.RowVersion != version {
		return CollectionPoint{}, ErrVersionMismatch
	}
	if cur.Status == "retired" {
		return CollectionPoint{}, ErrInvalidTransition
	}
	if cur.PublicKey != nil {
		if err := publickeys.Revoke(ctx, *cur.PublicKey); err != nil {
			return CollectionPoint{}, err
		}
	}
	if err := q.RetireCollectionPoint(ctx, id); err != nil {
		return CollectionPoint{}, err
	}
	if err := s.audit(ctx, "consent.collection_point.retire", CollectionPointType, id, map[string]any{"status": cur.Status}, map[string]any{"status": "retired"}); err != nil {
		return CollectionPoint{}, err
	}
	return s.GetCollectionPoint(ctx, id)
}

// ListCollectionPoints returns every point (without purposes).
func (s *Service) ListCollectionPoints(ctx context.Context) ([]CollectionPoint, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListCollectionPoints(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CollectionPoint, 0, len(rows))
	for _, r := range rows {
		cp := toCP(consentstore.GetCollectionPointRow(r))
		if cp.Purposes, err = cpPurposes(ctx, q, cp.ID); err != nil {
			return nil, err
		}
		out = append(out, cp)
	}
	return out, nil
}

// GetCollectionPoint returns a point with its purposes and their current versions.
func (s *Service) GetCollectionPoint(ctx context.Context, id uuid.UUID) (CollectionPoint, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.GetCollectionPoint(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return CollectionPoint{}, ErrNotFound
	}
	if err != nil {
		return CollectionPoint{}, err
	}
	cp := toCP(r)
	if cp.Purposes, err = cpPurposes(ctx, q, id); err != nil {
		return CollectionPoint{}, err
	}
	return cp, nil
}

// cpPurposes are a point's purposes with their current versions.
func cpPurposes(ctx context.Context, q *consentstore.Queries, id uuid.UUID) ([]CPPurpose, error) {
	ps, err := q.ListCollectionPointPurposes(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]CPPurpose, 0, len(ps))
	for _, p := range ps {
		out = append(out, CPPurpose{PurposeID: p.PurposeID, Code: p.Code, Name: Text{Th: p.NameTh, En: deref(p.NameEn)}, Status: p.Status,
			IsSensitive: p.IsSensitive, Required: p.IsRequired, CurrentVersionID: uuidPtr(p.CurrentVersionID), CurrentVersionNo: p.CurrentVersionNo,
			ConsentText: Text{Th: deref(p.TextTh), En: deref(p.TextEn)}, ExplicitText: Text{Th: deref(p.ExplicitTextTh), En: deref(p.ExplicitTextEn)},
			MinAge: p.MinAge, LifespanDays: p.LifespanDays})
	}
	return out, nil
}

func toCP(r consentstore.GetCollectionPointRow) CollectionPoint {
	cp := CollectionPoint{ID: r.ID, Code: r.Code, Name: r.Name, Channel: r.Channel, LegalEntityID: r.LegalEntityID, Status: r.Status,
		PublicKey: deref(r.PublicKey), AllowedOrigins: r.AllowedOrigins, RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time}
	if len(r.PublishChecklist) > 0 {
		cp.Checklist = &Checklist{}
		_ = json.Unmarshal(r.PublishChecklist, cp.Checklist)
	}
	if r.PublishedAt.Valid {
		t := r.PublishedAt.Time
		cp.PublishedAt = &t
	}
	if cp.AllowedOrigins == nil {
		cp.AllowedOrigins = []string{}
	}
	return cp
}
