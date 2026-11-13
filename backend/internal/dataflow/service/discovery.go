package service

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dataflowstore "pdpa-platform/internal/dataflow/store"
	orgservice "pdpa-platform/internal/org/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/connectors"
)

// DFG-09 (data discovery): connect a source, list its tables, sample a few rows per table and run every
// active classifier over each column's sampled values in-process. Only the match statistics (ratio and
// sample count) are stored — never a sampled value (rule 3).
const (
	// DiscoverySampleLimit bounds the rows pulled per table; DiscoveryMinMatchRatio is the share of a
	// column's non-empty values a classifier must match before a finding is proposed. Both are tunables.
	DiscoverySampleLimit   = 100
	DiscoveryMinMatchRatio = 0.5
)

var (
	ErrDiscoveryNotConfigured = errors.New("dataflow: connectors not configured")
	ErrDiscoveryNotFound      = errors.New("dataflow: discovery record not found")
	ErrDiscoveryVersion       = errors.New("dataflow: discovery finding version mismatch")
)

type DiscoveryScan struct {
	ID             uuid.UUID
	ConnectorID    uuid.UUID
	AssetID        *uuid.UUID
	Status         string
	StartedAt      *time.Time
	FinishedAt     *time.Time
	ObjectsScanned int32
	FindingsCount  int32
	RowVersion     int32
	CreatedAt      time.Time
}

type DiscoveryFinding struct {
	ID                  uuid.UUID
	ScanID              uuid.UUID
	ObjectPath          string
	ClassifierID        uuid.UUID
	ClassifierCode      string
	ClassifierName      string
	MatchRatio          float64
	SampleCount         int32
	SuggestedCategoryID *uuid.UUID
	Status              string
	RowVersion          int32
	CreatedAt           time.Time
}

type DiscoveryScanCursor struct {
	At time.Time
	ID uuid.UUID
}

// MatchValue reports whether one sampled value matches a classifier of the given type.
// checksum reuses ORG-01's Thai 13-digit mod-11 check; regex uses the classifier's own pattern;
// dictionary is a deliberately simple Thai-script heuristic (no real name dictionary is available).
func MatchValue(classifierType string, pattern *regexp.Regexp, value string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return false
	}
	switch classifierType {
	case "checksum":
		return orgservice.ValidThaiID(strings.NewReplacer("-", "", " ", "").Replace(v))
	case "regex":
		return pattern != nil && pattern.MatchString(strings.NewReplacer("-", "", " ", "").Replace(v))
	case "dictionary":
		words := strings.Fields(v)
		if len(words) < 2 {
			return false
		}
		for _, w := range words {
			n := 0
			for _, r := range w {
				if !unicode.Is(unicode.Thai, r) {
					return false
				}
				n++
			}
			if n < 2 {
				return false
			}
		}
		return true
	}
	return false
}

// RunScan runs a discovery scan synchronously inside the request transaction.
func (s *Service) RunScan(ctx context.Context, connectorID uuid.UUID, assetID *uuid.UUID) (DiscoveryScan, error) {
	if s.Connectors == nil {
		return DiscoveryScan{}, ErrDiscoveryNotConfigured
	}
	if _, err := s.Connectors.GetConnector(ctx, connectorID); err != nil {
		if errors.Is(err, connectors.ErrNotFound) {
			return DiscoveryScan{}, fmt.Errorf("%w: unknown connector", ErrInvalid)
		}
		return DiscoveryScan{}, err
	}
	if assetID != nil {
		if s.Ropa == nil {
			return DiscoveryScan{}, fmt.Errorf("%w: unknown asset", ErrInvalid)
		}
		if _, err := s.Ropa.GetAsset(ctx, *assetID); err != nil {
			return DiscoveryScan{}, fmt.Errorf("%w: unknown asset", ErrInvalid)
		}
	}
	q := dataflowstore.New(pdb.MustTxFromContext(ctx))
	classifiers, err := q.ListActiveClassifiers(ctx)
	if err != nil {
		return DiscoveryScan{}, err
	}
	type compiled struct {
		row dataflowstore.ListActiveClassifiersRow
		re  *regexp.Regexp
	}
	var cls []compiled
	for _, c := range classifiers {
		var re *regexp.Regexp
		if c.Pattern != nil && c.ClassifierType == "regex" {
			if re, err = regexp.Compile(*c.Pattern); err != nil {
				continue
			}
		}
		cls = append(cls, compiled{c, re})
	}

	scan, err := q.InsertDiscoveryScan(ctx, dataflowstore.InsertDiscoveryScanParams{ConnectorID: connectorID, AssetID: dfPgUUID(assetID)})
	if err != nil {
		return DiscoveryScan{}, err
	}

	status := "done"
	var objects, findings int32
	tables, derr := s.Connectors.Discover(ctx, connectorID)
	if derr != nil {
		status = "failed"
	}
	for _, t := range tables {
		rows, serr := s.Connectors.Sample(ctx, connectorID, t.Name, DiscoverySampleLimit)
		if serr != nil {
			status = "failed"
			continue
		}
		objects++
		for _, col := range t.Columns {
			var nonEmpty int
			hits := make([]int, len(cls))
			for _, r := range rows {
				v := strings.TrimSpace(r[col])
				if v == "" || v == "<nil>" {
					continue
				}
				nonEmpty++
				for i, c := range cls {
					if MatchValue(c.row.ClassifierType, c.re, v) {
						hits[i]++
					}
				}
			}
			if nonEmpty == 0 {
				continue
			}
			for i, c := range cls {
				ratio := float64(hits[i]) / float64(nonEmpty)
				if hits[i] == 0 || ratio < DiscoveryMinMatchRatio {
					continue
				}
				if _, err := q.InsertDiscoveryFinding(ctx, dataflowstore.InsertDiscoveryFindingParams{
					ScanID: scan.ID, ObjectPath: t.Name + "." + col, ClassifierID: c.row.ID,
					MatchRatio: ratioNumeric(ratio), SampleCount: int32(nonEmpty), SuggestedCategoryID: c.row.DataCategoryID,
				}); err != nil {
					return DiscoveryScan{}, err
				}
				findings++
			}
		}
	}
	done, err := q.FinishDiscoveryScan(ctx, dataflowstore.FinishDiscoveryScanParams{
		ID: scan.ID, Status: status, ObjectsScanned: &objects, FindingsCount: &findings,
	})
	if err != nil {
		return DiscoveryScan{}, err
	}
	return toDiscoveryScan(done.ID, done.ConnectorID, done.AssetID, done.Status, done.StartedAt, done.FinishedAt, done.ObjectsScanned, done.FindingsCount, done.RowVersion, done.CreatedAt), nil
}

func (s *Service) GetDiscoveryScan(ctx context.Context, id uuid.UUID) (DiscoveryScan, error) {
	r, err := dataflowstore.New(pdb.MustTxFromContext(ctx)).GetDiscoveryScan(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DiscoveryScan{}, ErrDiscoveryNotFound
	}
	if err != nil {
		return DiscoveryScan{}, err
	}
	return toDiscoveryScan(r.ID, r.ConnectorID, r.AssetID, r.Status, r.StartedAt, r.FinishedAt, r.ObjectsScanned, r.FindingsCount, r.RowVersion, r.CreatedAt), nil
}

func (s *Service) ListDiscoveryScans(ctx context.Context, cursor *DiscoveryScanCursor, limit int) ([]DiscoveryScan, *DiscoveryScanCursor, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	p := dataflowstore.ListDiscoveryScansParams{Lim: int32(limit + 1)}
	if cursor != nil {
		p.CursorAt = pgtype.Timestamptz{Time: cursor.At, Valid: true}
		p.CursorID = pgtype.UUID{Bytes: cursor.ID, Valid: true}
	}
	rows, err := dataflowstore.New(pdb.MustTxFromContext(ctx)).ListDiscoveryScans(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	var next *DiscoveryScanCursor
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next = &DiscoveryScanCursor{At: last.CreatedAt.Time, ID: last.ID}
	}
	out := make([]DiscoveryScan, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDiscoveryScan(r.ID, r.ConnectorID, r.AssetID, r.Status, r.StartedAt, r.FinishedAt, r.ObjectsScanned, r.FindingsCount, r.RowVersion, r.CreatedAt))
	}
	return out, next, nil
}

func (s *Service) ListDiscoveryFindings(ctx context.Context, scanID uuid.UUID) ([]DiscoveryFinding, error) {
	q := dataflowstore.New(pdb.MustTxFromContext(ctx))
	if _, err := q.GetDiscoveryScan(ctx, scanID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDiscoveryNotFound
		}
		return nil, err
	}
	rows, err := q.ListDiscoveryFindingsForScan(ctx, scanID)
	if err != nil {
		return nil, err
	}
	out := make([]DiscoveryFinding, 0, len(rows))
	for _, r := range rows {
		f := toDiscoveryFinding(r.ID, r.ScanID, r.ObjectPath, r.ClassifierID, r.MatchRatio, r.SampleCount, r.SuggestedCategoryID, r.Status, r.RowVersion, r.CreatedAt)
		f.ClassifierCode, f.ClassifierName = r.ClassifierCode, r.ClassifierName
		out = append(out, f)
	}
	return out, nil
}

// ReviewDiscoveryFinding accepts or rejects a proposed finding.
func (s *Service) ReviewDiscoveryFinding(ctx context.Context, id uuid.UUID, decision string, expectedRowVersion int32) (DiscoveryFinding, error) {
	if decision != "accepted" && decision != "rejected" {
		return DiscoveryFinding{}, fmt.Errorf("%w: decision", ErrInvalid)
	}
	q := dataflowstore.New(pdb.MustTxFromContext(ctx))
	if _, err := q.GetDiscoveryFinding(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DiscoveryFinding{}, ErrDiscoveryNotFound
		}
		return DiscoveryFinding{}, err
	}
	r, err := q.ReviewDiscoveryFinding(ctx, dataflowstore.ReviewDiscoveryFindingParams{ID: id, RowVersion: expectedRowVersion, Status: decision})
	if errors.Is(err, pgx.ErrNoRows) {
		return DiscoveryFinding{}, ErrDiscoveryVersion
	}
	if err != nil {
		return DiscoveryFinding{}, err
	}
	return toDiscoveryFinding(r.ID, r.ScanID, r.ObjectPath, r.ClassifierID, r.MatchRatio, r.SampleCount, r.SuggestedCategoryID, r.Status, r.RowVersion, r.CreatedAt), nil
}

func ratioNumeric(f float64) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(int64(f*100 + 0.5)), Exp: -2, Valid: true}
}

func toDiscoveryScan(id, connector uuid.UUID, asset pgtype.UUID, status string, started, finished pgtype.Timestamptz, objects, findings *int32, rv int32, created pgtype.Timestamptz) DiscoveryScan {
	d := DiscoveryScan{ID: id, ConnectorID: connector, AssetID: dfUUIDPtr(asset), Status: status, RowVersion: rv, CreatedAt: created.Time}
	if started.Valid {
		t := started.Time
		d.StartedAt = &t
	}
	if finished.Valid {
		t := finished.Time
		d.FinishedAt = &t
	}
	if objects != nil {
		d.ObjectsScanned = *objects
	}
	if findings != nil {
		d.FindingsCount = *findings
	}
	return d
}

func toDiscoveryFinding(id, scan uuid.UUID, path string, classifier uuid.UUID, ratio pgtype.Numeric, samples int32, cat pgtype.UUID, status string, rv int32, created pgtype.Timestamptz) DiscoveryFinding {
	f := DiscoveryFinding{ID: id, ScanID: scan, ObjectPath: path, ClassifierID: classifier, SampleCount: samples, SuggestedCategoryID: dfUUIDPtr(cat), Status: status, RowVersion: rv, CreatedAt: created.Time}
	if v, err := ratio.Float64Value(); err == nil {
		f.MatchRatio = v.Float64
	}
	return f
}
