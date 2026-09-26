package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	consentstore "pdpa-platform/internal/consent/store"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/versioning"
)

// Text is a string by language; th is required where the field is.
type Text struct {
	Th string `json:"th"`
	En string `json:"en,omitempty"`
}

// PreferenceOption is one choice of a purpose preference.
type PreferenceOption struct {
	Value string `json:"value"`
	Label Text   `json:"label"`
}

// Preference is a sub-choice of a purpose (channel, topic, frequency).
type Preference struct {
	Code    string             `json:"code"`
	Name    Text               `json:"name"`
	Type    string             `json:"type"` // channel | topic | frequency | other
	Options []PreferenceOption `json:"options"`
}

// PurposeContent is what a purpose version holds — the snapshot a draft carries through approval (PLT-08).
type PurposeContent struct {
	Name              Text         `json:"name"`
	Description       Text         `json:"description"`
	ConsentText       Text         `json:"consent_text"`
	ExplicitText      Text         `json:"explicit_text"` // required when a data category is sensitive (s.26, CON-10)
	DataCategoryCodes []string     `json:"data_category_codes"`
	MinAge            *int         `json:"min_age,omitempty"`
	LifespanDays      *int         `json:"lifespan_days,omitempty"`
	ChangeType        string       `json:"change_type"` // minor | material; the first version is "initial"
	RequiresReconsent bool         `json:"requires_reconsent"`
	Preferences       []Preference `json:"preferences"`
}

// PurposeVersion is a published version.
type PurposeVersion struct {
	ID                uuid.UUID
	No                int32
	ConsentText       Text
	ExplicitText      Text
	ChangeType        string
	RequiresReconsent bool
	PublishedAt       time.Time
}

// Purpose is a purpose with its live content and published versions (newest first).
type Purpose struct {
	ID               uuid.UUID
	Code             string
	LegalEntityID    uuid.UUID
	Status           string // draft (never published) | active | retired
	IsSensitive      bool
	RequiresExplicit bool
	LawfulBasis      string
	CurrentVersion   *PurposeVersion
	Live             PurposeContent // what the current version shows (empty before the first publish)
	Versions         []PurposeVersion
	RowVersion       int32
	UpdatedAt        time.Time
}

var codeRE = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{1,59}$`)
var keyRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,59}$`)

// RegisterVersioning puts purposes under PLT-08: drafts need a DPO's approval before they go live (maker-checker,
// security.md) and publishing applies the version in the same transaction.
func (s *Service) RegisterVersioning() {
	s.Versioning.Register(PurposeType, versioning.Policy{
		ReadPermission: "consent.purpose.read", EditPermission: "consent.purpose.update", PublishPermission: "consent.purpose.publish",
		Steps: []versioning.Step{{Role: "DPO"}},
		Title: func(ctx context.Context, id uuid.UUID) (string, error) {
			r, err := consentstore.New(pdb.MustTxFromContext(ctx)).GetPurpose(ctx, id)
			if err != nil {
				return "", err
			}
			return r.Code, nil
		},
		OnPublish: s.applyPurpose,
	})
}

// check validates content; sensitive says whether one of its data categories is (s.26).
func (s *Service) check(ctx context.Context, c *PurposeContent) (sensitive bool, err error) {
	trim := func(t *Text) { t.Th, t.En = strings.TrimSpace(t.Th), strings.TrimSpace(t.En) }
	trim(&c.Name)
	trim(&c.Description)
	trim(&c.ConsentText)
	trim(&c.ExplicitText)
	if c.Name.Th == "" || len([]rune(c.Name.Th)) > 200 || len([]rune(c.Name.En)) > 200 {
		return false, invalid("name")
	}
	if c.ConsentText.Th == "" || len([]rune(c.ConsentText.Th)) > 10000 || len([]rune(c.ConsentText.En)) > 10000 {
		return false, invalid("consent text")
	}
	if len([]rune(c.Description.Th)) > 2000 || len([]rune(c.Description.En)) > 2000 || len([]rune(c.ExplicitText.Th)) > 5000 || len([]rune(c.ExplicitText.En)) > 5000 {
		return false, invalid("text too long")
	}
	if c.MinAge != nil && (*c.MinAge < 0 || *c.MinAge > 25) {
		return false, invalid("min_age")
	}
	if c.LifespanDays != nil && (*c.LifespanDays < 1 || *c.LifespanDays > 3650) {
		return false, invalid("lifespan_days")
	}
	if c.ChangeType == "" {
		c.ChangeType = "minor"
	}
	if c.ChangeType != "minor" && c.ChangeType != "material" {
		return false, invalid("change_type")
	}
	if c.ChangeType != "material" {
		c.RequiresReconsent = false
	}
	if c.DataCategoryCodes == nil {
		c.DataCategoryCodes = []string{}
	}
	cats, err := s.Org.ListMaster(ctx, orgservice.KindDataCategories)
	if err != nil {
		return false, err
	}
	for _, code := range c.DataCategoryCodes {
		i := slices.IndexFunc(cats, func(m orgservice.MasterItem) bool { return m.Code == code })
		if i < 0 {
			return false, invalid("data category %q", code)
		}
		sensitive = sensitive || cats[i].IsSensitive
	}
	slices.Sort(c.DataCategoryCodes)
	c.DataCategoryCodes = slices.Compact(c.DataCategoryCodes)
	if c.Preferences == nil {
		c.Preferences = []Preference{}
	}
	seen := map[string]bool{}
	for i := range c.Preferences {
		p := &c.Preferences[i]
		trim(&p.Name)
		if !keyRE.MatchString(p.Code) || seen[p.Code] || p.Name.Th == "" || !slices.Contains([]string{"channel", "topic", "frequency", "other"}, p.Type) || len(p.Options) == 0 || len(p.Options) > 30 {
			return false, invalid("preference %q", p.Code)
		}
		seen[p.Code] = true
		vals := map[string]bool{}
		for j := range p.Options {
			o := &p.Options[j]
			trim(&o.Label)
			if !keyRE.MatchString(o.Value) || vals[o.Value] || o.Label.Th == "" {
				return false, invalid("preference %q option %q", p.Code, o.Value)
			}
			vals[o.Value] = true
		}
	}
	return sensitive, nil
}

// publishChecks are CON-10's rules at publish time: a purpose using sensitive data carries its own explicit
// statement (shown with its own checkbox).
func publishChecks(c PurposeContent, sensitive bool) []string {
	var failed []string
	if sensitive && c.ExplicitText.Th == "" {
		failed = append(failed, "explicit_text_required")
	}
	return failed
}

// CreatePurpose adds a purpose (never live until a version is approved and published) with its first draft.
func (s *Service) CreatePurpose(ctx context.Context, code string, legalEntity uuid.UUID, c PurposeContent) (Purpose, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !codeRE.MatchString(code) {
		return Purpose{}, invalid("code")
	}
	if _, err := s.check(ctx, &c); err != nil {
		return Purpose{}, err
	}
	if _, err := s.Org.GetLegalEntity(ctx, legalEntity); err != nil { // visible under RLS (FKs bypass it, rule 1)
		return Purpose{}, invalid("legal entity")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Purpose{}, err
	}
	err = pdb.Savepoint(ctx, func(ctx context.Context) error {
		return consentstore.New(pdb.MustTxFromContext(ctx)).InsertPurpose(ctx, consentstore.InsertPurposeParams{ID: id, Code: code, NameTh: c.Name.Th, NameEn: optText(c.Name.En), LegalEntityID: legalEntity})
	})
	if isUnique(err) {
		return Purpose{}, invalid("code exists")
	}
	if err != nil {
		return Purpose{}, err
	}
	if _, err := s.Versioning.SaveDraft(ctx, PurposeType, id, c); err != nil {
		return Purpose{}, err
	}
	if err := s.audit(ctx, "consent.purpose.create", PurposeType, id, nil, map[string]any{"code": code}); err != nil {
		return Purpose{}, err
	}
	return s.GetPurpose(ctx, id)
}

// SavePurposeDraft saves a draft of the next version (the open draft, or a new one after the last publish).
func (s *Service) SavePurposeDraft(ctx context.Context, id uuid.UUID, c PurposeContent) (versioning.Version, error) {
	r, err := consentstore.New(pdb.MustTxFromContext(ctx)).LockPurpose(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return versioning.Version{}, ErrNotFound
	}
	if err != nil {
		return versioning.Version{}, err
	}
	if r.Status == "retired" {
		return versioning.Version{}, ErrInvalidTransition
	}
	if _, err := s.check(ctx, &c); err != nil {
		return versioning.Version{}, err
	}
	return s.Versioning.SaveDraft(ctx, PurposeType, id, c)
}

// applyPurpose is the versioning OnPublish hook: the approved content becomes the purpose's new version.
func (s *Service) applyPurpose(ctx context.Context, id uuid.UUID, snapshot json.RawMessage) error {
	var c PurposeContent
	if err := json.Unmarshal(snapshot, &c); err != nil {
		return err
	}
	sensitive, err := s.check(ctx, &c)
	if err != nil {
		return err
	}
	if failed := publishChecks(c, sensitive); len(failed) > 0 {
		return &CheckError{Failed: failed}
	}
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	if _, err := q.LockPurpose(ctx, id); err != nil {
		return err
	}
	no, err := q.NextPurposeVersionNo(ctx, id)
	if err != nil {
		return err
	}
	changeType, reconsent := c.ChangeType, c.RequiresReconsent
	if no == 1 {
		changeType, reconsent = "initial", false
	}
	vid, err := uuid.NewV7()
	if err != nil {
		return err
	}
	if err := q.InsertPurposeVersion(ctx, consentstore.InsertPurposeVersionParams{ID: vid, PurposeID: id, VersionNo: no, TextTh: c.ConsentText.Th, TextEn: optText(c.ConsentText.En),
		ExplicitTextTh: optText(c.ExplicitText.Th), ExplicitTextEn: optText(c.ExplicitText.En), ChangeType: changeType, RequiresReconsent: reconsent,
		ApprovedBy: pgUUID(currentUser(ctx))}); err != nil {
		return err
	}
	basis := "CONSENT"
	if sensitive {
		basis = "EXPLICIT_CONSENT"
	}
	if err := q.ApplyPurpose(ctx, consentstore.ApplyPurposeParams{ID: id, NameTh: c.Name.Th, NameEn: optText(c.Name.En), DescriptionTh: optText(c.Description.Th),
		DescriptionEn: optText(c.Description.En), LawfulBasisCode: basis, IsSensitive: sensitive, RequiresExplicit: sensitive, MinAge: optInt16(c.MinAge),
		LifespanDays: optInt32(c.LifespanDays), DataCategoryCodes: c.DataCategoryCodes, CurrentVersionID: pgUUIDv(vid)}); err != nil {
		return err
	}
	if err := q.DeletePurposePreferences(ctx, id); err != nil {
		return err
	}
	for i, p := range c.Preferences {
		opts, _ := json.Marshal(p.Options)
		if err := q.InsertPurposePreference(ctx, consentstore.InsertPurposePreferenceParams{PurposeID: id, Code: p.Code, NameTh: p.Name.Th, NameEn: optText(p.Name.En),
			PrefType: p.Type, Options: opts, DisplayOrder: int16(i)}); err != nil {
			return err
		}
	}
	return s.audit(ctx, "consent.purpose.publish", PurposeType, id, nil, map[string]any{"version": no, "change_type": changeType, "requires_reconsent": reconsent, "sensitive": sensitive})
}

// ListPurposes returns every purpose with its live state (no versions).
func (s *Service) ListPurposes(ctx context.Context) ([]Purpose, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListPurposes(ctx)
	if err != nil {
		return nil, err
	}
	gs := make([]consentstore.GetPurposeRow, 0, len(rows))
	for _, r := range rows {
		gs = append(gs, consentstore.GetPurposeRow(r))
	}
	return withDetails(ctx, q, gs)
}

// GetPurpose returns a purpose with its published versions.
func (s *Service) GetPurpose(ctx context.Context, id uuid.UUID) (Purpose, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.GetPurpose(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Purpose{}, ErrNotFound
	}
	if err != nil {
		return Purpose{}, err
	}
	ps, err := withDetails(ctx, q, []consentstore.GetPurposeRow{r})
	if err != nil {
		return Purpose{}, err
	}
	return ps[0], nil
}

// withDetails builds purposes with their published versions, live texts and preferences.
func withDetails(ctx context.Context, q *consentstore.Queries, rows []consentstore.GetPurposeRow) ([]Purpose, error) {
	out := make([]Purpose, 0, len(rows))
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		p := toPurpose(r)
		vs, err := q.ListPurposeVersions(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		for _, v := range vs {
			pv := toVersion(consentstore.GetPurposeVersionRow(v))
			p.Versions = append(p.Versions, pv)
			if r.CurrentVersionID.Valid && v.ID == uuid.UUID(r.CurrentVersionID.Bytes) {
				cp := pv
				p.CurrentVersion = &cp
				p.Live.ConsentText, p.Live.ExplicitText = pv.ConsentText, pv.ExplicitText
				p.Live.ChangeType, p.Live.RequiresReconsent = pv.ChangeType, pv.RequiresReconsent
			}
		}
		p.Live.Preferences = []Preference{}
		out = append(out, p)
		ids = append(ids, r.ID)
	}
	prefs, err := q.ListPurposePreferences(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, pr := range prefs {
		for i := range out {
			if out[i].ID == pr.PurposeID {
				out[i].Live.Preferences = append(out[i].Live.Preferences, toPreference(pr))
			}
		}
	}
	return out, nil
}

// RetirePurpose takes a purpose out of use; refused while an active collection point still shows it. Consent
// already given stays on record.
func (s *Service) RetirePurpose(ctx context.Context, id uuid.UUID) error {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.LockPurpose(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if r.Status == "retired" {
		return ErrInvalidTransition
	}
	n, err := q.CountPurposeUse(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrInUse
	}
	if err := q.SetPurposeStatus(ctx, consentstore.SetPurposeStatusParams{ID: id, Status: "retired"}); err != nil {
		return err
	}
	return s.audit(ctx, "consent.purpose.retire", PurposeType, id, map[string]any{"status": r.Status}, map[string]any{"status": "retired"})
}

func toPurpose(r consentstore.GetPurposeRow) Purpose {
	p := Purpose{ID: r.ID, Code: r.Code, LegalEntityID: r.LegalEntityID, Status: r.Status, IsSensitive: r.IsSensitive, RequiresExplicit: r.RequiresExplicit,
		LawfulBasis: r.LawfulBasisCode, RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time}
	p.Live.Name = Text{Th: r.NameTh, En: deref(r.NameEn)}
	p.Live.Description = Text{Th: deref(r.DescriptionTh), En: deref(r.DescriptionEn)}
	p.Live.DataCategoryCodes = r.DataCategoryCodes
	if r.MinAge != nil {
		v := int(*r.MinAge)
		p.Live.MinAge = &v
	}
	if r.LifespanDays != nil {
		v := int(*r.LifespanDays)
		p.Live.LifespanDays = &v
	}
	if r.CurrentVersionNo != nil && r.CurrentVersionID.Valid {
		p.CurrentVersion = &PurposeVersion{ID: uuid.UUID(r.CurrentVersionID.Bytes), No: *r.CurrentVersionNo}
	}
	return p
}

func toVersion(v consentstore.GetPurposeVersionRow) PurposeVersion {
	return PurposeVersion{ID: v.ID, No: v.VersionNo, ConsentText: Text{Th: v.TextTh, En: deref(v.TextEn)}, ExplicitText: Text{Th: deref(v.ExplicitTextTh), En: deref(v.ExplicitTextEn)},
		ChangeType: v.ChangeType, RequiresReconsent: v.RequiresReconsent, PublishedAt: v.PublishedAt.Time}
}

func toPreference(p consentstore.ListPurposePreferencesRow) Preference {
	pr := Preference{Code: p.Code, Name: Text{Th: p.NameTh, En: deref(p.NameEn)}, Type: p.PrefType}
	_ = json.Unmarshal(p.Options, &pr.Options)
	if pr.Options == nil {
		pr.Options = []PreferenceOption{}
	}
	return pr
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
