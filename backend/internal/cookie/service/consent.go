package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/google/uuid"

	cookiestore "pdpa-platform/internal/cookie/store"
	pdb "pdpa-platform/internal/pkg/db"
)

var consentActions = []string{"accept_all", "reject_all", "custom"}

// RecordConsentInput is one visitor's banner decision — the acceptance criterion's own evidence
// ("ปฏิเสธทั้งหมดได้ในคลิกเดียว" means exactly one call with Action: "reject_all", no further round trip).
type RecordConsentInput struct {
	DomainID       uuid.UUID
	VisitorID      string          // the SDK's own first-party cookie value
	BannerConfigID uuid.UUID       // the published version the visitor was actually shown
	Action         string          // accept_all | reject_all | custom
	Choices        map[string]bool // category code -> on/off; required (and only meaningful) for "custom"
	IP             netip.Addr
	UserAgent      string
}

// RecordConsent is the acceptance criterion's own evidence write — append-only (CLAUDE.md rule 4's own
// spirit, even though the rule names only consent.*/breach.*/platform.audit_log explicitly: no update or
// delete query exists for cookie.consent_records anywhere in this module). The IP is hashed, never stored
// in the clear (rule 3) — this table only ever had an ip_hash column to begin with.
func (s *Service) RecordConsent(ctx context.Context, in RecordConsentInput) error {
	if !slices.Contains(consentActions, in.Action) {
		return fmt.Errorf("%w: action", ErrInvalid)
	}
	if strings.TrimSpace(in.VisitorID) == "" {
		return fmt.Errorf("%w: visitor_id", ErrInvalid)
	}
	if _, err := s.GetBannerConfig(ctx, in.BannerConfigID); err != nil {
		return fmt.Errorf("%w: banner_config_id", ErrInvalid)
	}
	choices := in.Choices
	if choices == nil {
		choices = map[string]bool{}
	}
	choicesJSON, err := json.Marshal(choices)
	if err != nil {
		return err
	}
	var ipHash *string
	if in.IP.IsValid() {
		sum := sha256.Sum256([]byte(in.IP.String()))
		h := hex.EncodeToString(sum[:])
		ipHash = &h
	}
	var ua *string
	if in.UserAgent != "" {
		ua = &in.UserAgent
	}
	if err := cookiestore.New(pdb.MustTxFromContext(ctx)).InsertConsentRecord(ctx, cookiestore.InsertConsentRecordParams{
		DomainID: in.DomainID, VisitorID: in.VisitorID, BannerConfigID: in.BannerConfigID,
		Action: in.Action, Choices: choicesJSON, IpHash: ipHash, UserAgent: ua,
	}); err != nil {
		return err
	}
	return s.audit(ctx, "cookie.consent.record", DomainEntityType, in.DomainID, nil, map[string]any{"action": in.Action})
}
