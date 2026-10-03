package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	auditstore "pdpa-platform/internal/platform/audit/store"
)

// ORG-19: searching and exporting the tenant's audit trail. Reads run in the caller's tenant transaction,
// so RLS limits them to that tenant; nobody can change a row (PLT-12).

// Filter narrows a search. Kind is "changes" (business actions, the default), "requests" (one row per
// API request) or "all". ActionPrefix matches the start of the action, e.g. "iam." for one module.
type Filter struct {
	ActorID      *uuid.UUID
	EntityType   string
	EntityID     *uuid.UUID
	ActionPrefix string
	From, To     *time.Time
	Kind         string
}

// LogEntry is one audit row with the names of the people involved.
type LogEntry struct {
	ID         int64
	OccurredAt time.Time
	ActorType  string
	ActorID    *uuid.UUID
	ActorName  string
	Action     string
	EntityType string
	EntityID   *uuid.UUID
	EntityName string // for entity_type "user": whose record it is
	Before     any
	After      any
	IP         *netip.Addr
	UserAgent  string
}

var (
	ErrBadCursor      = errors.New("audit: bad cursor")
	ErrBadFilter      = errors.New("audit: bad filter")
	ErrExportTooLarge = errors.New("audit: too many rows to export")
)

// MaxExportRows bounds a synchronous export (config); narrow the filter for more.
const MaxExportRows = 50000

// Search returns up to limit entries newest first, and the cursor of the next page ("" at the end).
func (s *Service) Search(ctx context.Context, f Filter, cursor string, limit int) ([]LogEntry, string, error) {
	p, err := searchParams(f)
	if err != nil {
		return nil, "", err
	}
	if cursor != "" {
		if p.BeforeOccurredAt, p.BeforeID, err = decodeCursor(cursor); err != nil {
			return nil, "", err
		}
	}
	p.PageSize = int32(limit + 1)
	rows, err := auditstore.New(pdb.MustTxFromContext(ctx)).SearchAuditLog(ctx, p)
	if err != nil {
		return nil, "", fmt.Errorf("audit: search: %w", err)
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = encodeCursor(last.OccurredAt.Time, last.ID)
	}
	out, err := toEntries(ctx, rows)
	return out, next, err
}

// Export writes every entry matching f as CSV (UTF-8 with BOM for Excel), newest first, and records the
// export itself in the audit trail. More than MaxExportRows is refused before anything is written.
func (s *Service) Export(ctx context.Context, f Filter) (*bytes.Buffer, int, error) {
	p, err := searchParams(f)
	if err != nil {
		return nil, 0, err
	}
	q := auditstore.New(pdb.MustTxFromContext(ctx))
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"occurred_at", "actor_type", "actor_id", "actor_name", "action", "entity_type", "entity_id", "entity_name", "before", "after", "ip", "user_agent"})
	n := 0
	for {
		p.PageSize = 1000
		rows, err := q.SearchAuditLog(ctx, p)
		if err != nil {
			return nil, 0, fmt.Errorf("audit: export: %w", err)
		}
		if n+len(rows) > MaxExportRows {
			return nil, 0, ErrExportTooLarge
		}
		entries, err := toEntries(ctx, rows)
		if err != nil {
			return nil, 0, err
		}
		for _, e := range entries {
			_ = w.Write([]string{e.OccurredAt.UTC().Format(time.RFC3339Nano), e.ActorType, uuidString(e.ActorID), cell(e.ActorName), cell(e.Action),
				cell(e.EntityType), uuidString(e.EntityID), cell(e.EntityName), jsonCell(e.Before), jsonCell(e.After), ipString(e.IP), cell(e.UserAgent)})
		}
		n += len(rows)
		if len(rows) < int(p.PageSize) {
			break
		}
		last := rows[len(rows)-1]
		p.BeforeOccurredAt, p.BeforeID = last.OccurredAt, last.ID
	}
	w.Flush()
	if err := s.writeExportAudit(ctx, f, n); err != nil {
		return nil, 0, err
	}
	return &buf, n, nil
}

func (s *Service) writeExportAudit(ctx context.Context, f Filter, rows int) error {
	var tenant string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&tenant); err != nil {
		return err
	}
	e := Entry{ActorType: "system", Action: "platform.audit.export", After: map[string]any{"filter": filterAudit(f), "rows": rows}}
	e.TenantID, _ = uuid.Parse(tenant)
	var user string
	_ = pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.user_id', true)`).Scan(&user)
	if u, err := uuid.Parse(user); err == nil {
		e.ActorType, e.ActorID = "user", &u
	}
	return s.Write(ctx, e)
}

func filterAudit(f Filter) map[string]any {
	m := map[string]any{"kind": f.Kind}
	if f.ActorID != nil {
		m["actor_id"] = f.ActorID
	}
	if f.EntityType != "" {
		m["entity_type"] = f.EntityType
	}
	if f.EntityID != nil {
		m["entity_id"] = f.EntityID
	}
	if f.ActionPrefix != "" {
		m["action_prefix"] = f.ActionPrefix
	}
	if f.From != nil {
		m["from"] = f.From.UTC()
	}
	if f.To != nil {
		m["to"] = f.To.UTC()
	}
	return m
}

func searchParams(f Filter) (auditstore.SearchAuditLogParams, error) {
	p := auditstore.SearchAuditLogParams{Kind: f.Kind,
		BeforeOccurredAt: pgtype.Timestamptz{InfinityModifier: pgtype.Infinity, Valid: true}, BeforeID: 1<<63 - 1}
	switch f.Kind {
	case "":
		p.Kind = "changes"
	case "changes", "requests", "all":
	default:
		return p, fmt.Errorf("%w: kind", ErrBadFilter)
	}
	if f.ActorID != nil {
		p.ActorID = pgtype.UUID{Bytes: *f.ActorID, Valid: true}
	}
	if f.EntityID != nil {
		p.EntityID = pgtype.UUID{Bytes: *f.EntityID, Valid: true}
	}
	if f.EntityType != "" {
		p.EntityType = &f.EntityType
	}
	if f.ActionPrefix != "" {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(f.ActionPrefix)
		p.ActionPrefix = &esc
	}
	if f.From != nil {
		p.FromAt = pgtype.Timestamptz{Time: *f.From, Valid: true}
	}
	if f.To != nil {
		p.ToAt = pgtype.Timestamptz{Time: *f.To, Valid: true}
	}
	if f.From != nil && f.To != nil && !f.To.After(*f.From) {
		return p, fmt.Errorf("%w: to must be after from", ErrBadFilter)
	}
	return p, nil
}

func toEntries(ctx context.Context, rows []auditstore.SearchAuditLogRow) ([]LogEntry, error) {
	var ids []uuid.UUID
	for _, r := range rows {
		if r.ActorID.Valid {
			ids = append(ids, uuid.UUID(r.ActorID.Bytes))
		}
		if r.EntityID.Valid && r.EntityType != nil && *r.EntityType == "user" {
			ids = append(ids, uuid.UUID(r.EntityID.Bytes))
		}
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]LogEntry, 0, len(rows))
	for _, r := range rows {
		e := LogEntry{ID: r.ID, OccurredAt: r.OccurredAt.Time, ActorType: r.ActorType, ActorID: optUUID(r.ActorID), Action: r.Action,
			EntityID: optUUID(r.EntityID), IP: r.Ip}
		if e.ActorID != nil {
			e.ActorName = names[*e.ActorID]
		}
		if r.EntityType != nil {
			e.EntityType = *r.EntityType
			if e.EntityType == "user" && e.EntityID != nil {
				e.EntityName = names[*e.EntityID]
			}
		}
		if r.UserAgent != nil {
			e.UserAgent = *r.UserAgent
		}
		if len(r.Before) > 0 {
			_ = json.Unmarshal(r.Before, &e.Before)
		}
		if len(r.After) > 0 {
			_ = json.Unmarshal(r.After, &e.After)
		}
		out = append(out, e)
	}
	return out, nil
}

func encodeCursor(t time.Time, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(id, 10)))
}

func decodeCursor(c string) (pgtype.Timestamptz, int64, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return pgtype.Timestamptz{}, 0, ErrBadCursor
	}
	ts, idStr, ok := strings.Cut(string(b), "|")
	t, err1 := time.Parse(time.RFC3339Nano, ts)
	id, err2 := strconv.ParseInt(idStr, 10, 64)
	if !ok || err1 != nil || err2 != nil {
		return pgtype.Timestamptz{}, 0, ErrBadCursor
	}
	return pgtype.Timestamptz{Time: t, Valid: true}, id, nil
}

// cell guards against spreadsheet formula injection: a value starting with = + - @ (or a tab/CR) is
// prefixed with a quote so Excel shows it as text.
func cell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func jsonCell(v any) string {
	if v == nil {
		return ""
	}
	b, _ := json.Marshal(v)
	return cell(string(b))
}

func uuidString(u *uuid.UUID) string {
	if u == nil {
		return ""
	}
	return u.String()
}

func ipString(ip *netip.Addr) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}
