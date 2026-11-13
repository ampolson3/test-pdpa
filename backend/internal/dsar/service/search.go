package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	dsarstore "pdpa-platform/internal/dsar/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/connectors"
)

// SearchResult is DSAR-09's own read-side view of one dsar.search_results row — never the decrypted
// summary (rule 3): a staff member sees whether a system had data and roughly how much, then acts through
// that system's own tools, not through this one.
type SearchResult struct {
	ID            uuid.UUID
	ConnectorID   uuid.UUID
	ConnectorName string
	ConnectorType string
	Found         bool
	RecordCount   *int32
	SearchedAt    time.Time
	PurgedAt      *time.Time
}

func toSearchResult(r dsarstore.ListSearchResultsForRequestRow) SearchResult {
	out := SearchResult{
		ID: r.ID, ConnectorID: r.ConnectorID, ConnectorName: r.ConnectorName, ConnectorType: r.ConnectorType,
		Found: r.Found, RecordCount: r.RecordCount, SearchedAt: r.SearchedAt.Time,
	}
	if r.PurgedAt.Valid {
		t := r.PurgedAt.Time
		out.PurgedAt = &t
	}
	return out
}

// SearchAcrossSystems is the acceptance criterion's own first half, "ค้นข้อมูลจากระบบที่เชื่อมได้ในคำสั่ง
// เดียว": one call searches every one of the tenant's active, search-capable connectors (PLT-20) for the
// request's own requester contact, recording one dsar.search_results row per connector. A single
// connector's own failure (unreachable, misconfigured) is recorded as found=false and never aborts the
// rest of the run — the literal "ในคำสั่งเดียว" is about issuing one search command, not requiring every
// system to answer.
func (s *Service) SearchAcrossSystems(ctx context.Context, requestID uuid.UUID) ([]SearchResult, error) {
	if s.Connectors == nil {
		return nil, fmt.Errorf("%w: connectors not configured", ErrInvalid)
	}
	if _, err := s.GetRequest(ctx, requestID); err != nil {
		return nil, err
	}
	identifier, err := s.decryptRequesterContact(ctx, requestID)
	if err != nil {
		return nil, err
	}

	searchable, err := s.Connectors.ListActiveWithCapability(ctx, connectors.CapSearchSubject)
	if err != nil {
		return nil, err
	}

	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	for _, c := range searchable {
		records, searchErr := s.Connectors.SearchSubject(ctx, c.ID, identifier)
		found := searchErr == nil && len(records) > 0
		var recordCount *int32
		var summaryEnc []byte
		if found {
			n := int32(len(records))
			recordCount = &n
			body, err := json.Marshal(records)
			if err != nil {
				return nil, err
			}
			summaryEnc, err = s.Keyring.Encrypt(ctx, cryptoClass, searchResultContext(requestID, c.ID), body)
			if err != nil {
				return nil, err
			}
		}
		if _, err := q.InsertSearchResult(ctx, dsarstore.InsertSearchResultParams{
			RequestID: requestID, ConnectorID: c.ID, Found: found, RecordCount: recordCount, SummaryEnc: summaryEnc,
		}); err != nil {
			return nil, err
		}
	}

	if err := s.audit(ctx, "dsar.request.search", requestID, nil, map[string]any{"connectors_searched": len(searchable)}); err != nil {
		return nil, err
	}
	return s.ListSearchResults(ctx, requestID)
}

// ListSearchResults is the read side, newest first.
func (s *Service) ListSearchResults(ctx context.Context, requestID uuid.UUID) ([]SearchResult, error) {
	if _, err := s.GetRequest(ctx, requestID); err != nil {
		return nil, err
	}
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListSearchResultsForRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSearchResult(r))
	}
	return out, nil
}

// purgeSearchResults is DSAR-09's own other acceptance-criterion half, "ผลถูกลบเมื่อปิดคำขอ" — called
// from Transition the moment a request reaches a terminal status. The row stays (so staff can still see
// which systems were searched and whether anything was found), only the encrypted content is wiped.
func (s *Service) purgeSearchResults(ctx context.Context, requestID uuid.UUID) error {
	return dsarstore.New(pdb.MustTxFromContext(ctx)).PurgeSearchResultsForRequest(ctx, requestID)
}

func searchResultContext(requestID, connectorID uuid.UUID) string {
	return "dsar.search_results:" + requestID.String() + ":" + connectorID.String()
}
