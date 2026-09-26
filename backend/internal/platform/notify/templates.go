package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	pdb "pdpa-platform/internal/pkg/db"
	notifystore "pdpa-platform/internal/platform/notify/store"
)

// Template is a notification template. Global templates (Global) are provided by the platform and read-only
// for tenants; a tenant overrides one by creating its own with the same code, channel and language.
type Template struct {
	ID         uuid.UUID
	Global     bool
	Code       string
	Channel    string
	Language   string
	Subject    string
	Body       string
	Variables  []string
	RowVersion int32
	UpdatedAt  time.Time
}

// TemplateInput is what an admin edits.
type TemplateInput struct {
	Code      string
	Channel   string
	Language  string
	Subject   string
	Body      string
	Variables []string
}

var (
	ErrDuplicateTemplate = errors.New("notify: a template with this code, channel and language exists")
	codeRE               = regexp.MustCompile(`^[a-z][a-z0-9_.]{1,79}$`)
	varRE                = regexp.MustCompile(`^[a-z][a-z0-9_]{0,59}$`)
)

// validateInput checks the fields and that the template renders with its declared variables only.
func validateInput(in TemplateInput, checkKey bool) error {
	if checkKey {
		if !codeRE.MatchString(in.Code) || !validChannel(in.Channel) || (in.Language != "th" && in.Language != "en") {
			return fmt.Errorf("%w: code, channel or language", ErrInvalidRequest)
		}
	}
	if in.Body == "" {
		return fmt.Errorf("%w: body is required", ErrInvalidRequest)
	}
	for _, v := range in.Variables {
		if !varRE.MatchString(v) {
			return fmt.Errorf("%w: variable %q", ErrInvalidRequest, v)
		}
	}
	_, err := Preview(in, nil)
	return err
}

// Preview renders a template with vars, filling any declared variable vars lacks with a placeholder
// ({name}), so an editor can see the result before saving.
func Preview(in TemplateInput, vars map[string]any) (Rendered, error) {
	all := sampleVars(in.Variables)
	for k, v := range vars {
		all[k] = v
	}
	var subject *string
	if in.Subject != "" {
		subject = &in.Subject
	}
	return render(subject, in.Body, all)
}

func (s *Service) ListTemplates(ctx context.Context) ([]Template, error) {
	rows, err := notifystore.New(pdb.MustTxFromContext(ctx)).ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Template, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTemplate(notifystore.GetTemplateRow(r)))
	}
	return out, nil
}

func (s *Service) GetTemplate(ctx context.Context, id uuid.UUID) (Template, error) {
	r, err := notifystore.New(pdb.MustTxFromContext(ctx)).GetTemplate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	return toTemplate(r), nil
}

func (s *Service) CreateTemplate(ctx context.Context, in TemplateInput) (Template, error) {
	if err := validateInput(in, true); err != nil {
		return Template{}, err
	}
	vars, _ := json.Marshal(nonNil(in.Variables))
	r, err := notifystore.New(pdb.MustTxFromContext(ctx)).InsertTemplate(ctx, notifystore.InsertTemplateParams{
		Code: in.Code, Channel: in.Channel, Language: in.Language, Subject: optional(in.Subject), Body: in.Body, Variables: vars,
	})
	if errors.Is(err, pgx.ErrNoRows) { // ON CONFLICT DO NOTHING: the tenant already has this template
		return Template{}, ErrDuplicateTemplate
	}
	if err != nil {
		return Template{}, err
	}
	return toTemplate(notifystore.GetTemplateRow(r)), nil
}

// UpdateTemplate changes subject, body and variables of the tenant's own template, if version still
// matches (If-Match). Code, channel and language identify the template and don't change.
func (s *Service) UpdateTemplate(ctx context.Context, id uuid.UUID, version int32, in TemplateInput) (Template, error) {
	if err := validateInput(in, false); err != nil {
		return Template{}, err
	}
	vars, _ := json.Marshal(nonNil(in.Variables))
	r, err := notifystore.New(pdb.MustTxFromContext(ctx)).UpdateTemplate(ctx, notifystore.UpdateTemplateParams{
		ID: id, RowVersion: version, Subject: optional(in.Subject), Body: in.Body, Variables: vars,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, s.missOrStale(ctx, id)
	}
	if err != nil {
		return Template{}, err
	}
	return toTemplate(notifystore.GetTemplateRow(r)), nil
}

func (s *Service) DeleteTemplate(ctx context.Context, id uuid.UUID, version int32) error {
	n, err := notifystore.New(pdb.MustTxFromContext(ctx)).DeleteTemplate(ctx, notifystore.DeleteTemplateParams{ID: id, RowVersion: version})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return fmt.Errorf("%w: template is referenced by sent notifications", ErrInvalidRequest)
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return s.missOrStale(ctx, id)
	}
	return nil
}

// missOrStale tells a stale If-Match (the tenant's template exists at another version) from a template
// the tenant can't change (missing, another tenant's, or global).
func (s *Service) missOrStale(ctx context.Context, id uuid.UUID) error {
	t, err := s.GetTemplate(ctx, id)
	if err != nil || t.Global {
		return ErrNotFound
	}
	return ErrVersionMismatch
}

func toTemplate(r notifystore.GetTemplateRow) Template {
	t := Template{
		ID: r.ID, Global: !r.TenantID.Valid, Code: r.Code, Channel: r.Channel, Language: r.Language,
		Body: r.Body, Variables: parseVariables(r.Variables), RowVersion: r.RowVersion, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.Subject != nil {
		t.Subject = *r.Subject
	}
	return t
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
