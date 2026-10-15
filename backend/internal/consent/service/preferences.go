package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	consentstore "pdpa-platform/internal/consent/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/publickeys"
)

// ---- CON-18: self-service preference centre ----
//
// There is no dedicated "preference center" table: the public key stands directly for an org.legal_entities
// row (publickeys.EntityPreferenceCenter == "portal", already in platform.public_keys' own CHECK constraint,
// unused until now) — one preference centre per legal entity, the same granularity CON-09's own collection
// points already use. Unlike a collection point, the preference centre shows every active purpose of the
// tenant a subject may have a decision on, not one form's own fixed list, and — ม.19's own requirement —
// lets them withdraw as easily as they gave: Record()'s own "a public submission can't withdraw" rule
// (CON-19) simply doesn't apply here, since every call here is OTP-verified first (CON-19's own identity gate,
// reused with Purpose "preference_center" instead of "consent" so a plain consent-form OTP can never double
// as a preference-centre session).

// PreferenceItem is one purpose as the preference centre shows it, in one language.
type PreferenceItem struct {
	Code         string
	VersionNo    int32
	Name         string
	Description  string
	Text         string
	ExplicitText string
	IsSensitive  bool
	Decision     string // ACTIVE | NOT_GIVEN | WITHDRAWN | EXPIRED | "" (never decided)
	Preferences  []PublicPreference
	Current      json.RawMessage // the subject's own saved preference choices, if any
	ExpiresAt    *time.Time
}

// PreferenceCenter is what GetPreferenceCenter returns after the OTP gate passes.
type PreferenceCenter struct {
	Identifiers []MaskedIdentifier
	Purposes    []PreferenceItem
}

// PublishPreferenceCenter issues the legal entity's preference-centre link on first use; a later call returns
// the same key (coalesce-on-first-publish, the same pattern DSAR-01/PNG-06/BRE-01/CON-01 already use).
func (s *Service) PublishPreferenceCenter(ctx context.Context, legalEntityID uuid.UUID) (string, error) {
	if _, err := s.Org.GetLegalEntity(ctx, legalEntityID); err != nil {
		return "", invalid("legal entity")
	}
	if key, ok, err := publickeys.Get(ctx, publickeys.EntityPreferenceCenter, legalEntityID); err != nil {
		return "", err
	} else if ok {
		return key, nil
	}
	key, err := publickeys.Issue(ctx, publickeys.EntityPreferenceCenter, legalEntityID, nil)
	if err != nil {
		return "", err
	}
	if err := s.audit(ctx, "consent.preference_center.publish", "legal_entity", legalEntityID, nil, map[string]any{"key_issued": true}); err != nil {
		return "", err
	}
	return key, nil
}

// RetirePreferenceCenter stops the legal entity's current link from resolving; publishing again issues a new one.
func (s *Service) RetirePreferenceCenter(ctx context.Context, legalEntityID uuid.UUID) error {
	key, ok, err := publickeys.Get(ctx, publickeys.EntityPreferenceCenter, legalEntityID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	if err := publickeys.Revoke(ctx, key); err != nil {
		return err
	}
	return s.audit(ctx, "consent.preference_center.retire", "legal_entity", legalEntityID, nil, nil)
}

// GetPreferenceCenterLink reports whether the legal entity already has an active link, and its key.
func (s *Service) GetPreferenceCenterLink(ctx context.Context, legalEntityID uuid.UUID) (string, bool, error) {
	if _, err := s.Org.GetLegalEntity(ctx, legalEntityID); err != nil {
		return "", false, invalid("legal entity")
	}
	return publickeys.Get(ctx, publickeys.EntityPreferenceCenter, legalEntityID)
}

// StartPreferenceVerification sends an OTP to the identifier the preference-centre page collected — the same
// IAM-05 primitive CON-19 uses, under its own purpose ("preference_center") so this OTP can't be reused to
// satisfy a plain consent-form's own CON-19 gate, or vice versa.
func (s *Service) StartPreferenceVerification(ctx context.Context, id Identifier) (ConsentVerificationRef, error) {
	kind := crypto.IdentifierKind(id.Type)
	method, ok := otpMethodFor(kind)
	if !ok {
		return ConsentVerificationRef{}, invalid("identifier")
	}
	v, err := crypto.Normalize(kind, id.Value)
	if err != nil {
		return ConsentVerificationRef{}, invalid("identifier")
	}
	if s.Verification == nil {
		return ConsentVerificationRef{}, fmt.Errorf("consent: verification not wired")
	}
	out, err := s.Verification.StartVerification(ctx, iamservice.StartVerificationInput{Purpose: "preference_center", Method: method, IdentifierKind: kind, IdentifierValue: v})
	if err != nil {
		return ConsentVerificationRef{}, err
	}
	channel := "sms"
	if method == "otp_email" {
		channel = "email"
	}
	return ConsentVerificationRef{ID: out.ID, Channel: channel}, nil
}

// verifyPreferenceIdentity is the preference centre's own identity gate. code is the OTP the subject was just
// sent; once a verification has already been decided "verified" (from an earlier call in the same session —
// VerifyOTP refuses to re-check a decided row), an empty or stale code is fine as long as the verification
// still matches this exact identifier — so a page can call GetPreferenceCenter once with the code, then
// UpdatePreferences again without asking the subject to retype it (VerifyOTP's own 5-attempt cap would
// otherwise be spent twice for one legitimate code).
func (s *Service) verifyPreferenceIdentity(ctx context.Context, verificationID uuid.UUID, id Identifier, code string) error {
	if s.Verification == nil {
		return fmt.Errorf("consent: verification not wired")
	}
	kind := crypto.IdentifierKind(id.Type)
	v, err := crypto.Normalize(kind, id.Value)
	if err != nil {
		return invalid("identifier")
	}
	if code != "" {
		if _, err := s.Verification.VerifyOTP(ctx, verificationID, code); err != nil && !errors.Is(err, iamservice.ErrVerificationDecided) {
			return &DecisionError{Fields: []FieldError{{"verification", "invalid"}}}
		}
	}
	ok, err := s.Verification.VerifiedIdentifier(ctx, verificationID, "preference_center", kind, v)
	if err != nil {
		return err
	}
	if !ok {
		return &DecisionError{Fields: []FieldError{{"verification", "invalid"}}}
	}
	return nil
}

// activePreferencePurposes builds the CPPurpose shape checkDecisions/writeDecisions already know how to
// validate against, from every active published purpose in the tenant rather than one collection point's own
// list — the preference centre lets a subject act on any purpose they may have a decision on.
func (s *Service) activePreferencePurposes(ctx context.Context) ([]Purpose, []CPPurpose, error) {
	purposes, err := s.ListPurposes(ctx)
	if err != nil {
		return nil, nil, err
	}
	var cps []CPPurpose
	for _, p := range purposes {
		if p.Status != "active" || p.CurrentVersion == nil {
			continue
		}
		var minAge *int16
		if p.Live.MinAge != nil {
			v := int16(*p.Live.MinAge)
			minAge = &v
		}
		var lifespan *int32
		if p.Live.LifespanDays != nil {
			v := int32(*p.Live.LifespanDays)
			lifespan = &v
		}
		vid, vno := p.CurrentVersion.ID, p.CurrentVersion.No
		cps = append(cps, CPPurpose{PurposeID: p.ID, Code: p.Code, Name: p.Live.Name, Status: p.Status, IsSensitive: p.IsSensitive,
			CurrentVersionID: &vid, CurrentVersionNo: &vno, ConsentText: p.Live.ConsentText, ExplicitText: p.Live.ExplicitText, MinAge: minAge, LifespanDays: lifespan})
	}
	return purposes, cps, nil
}

// GetPreferenceCenter is what the preference-centre page shows once the OTP gate passes: every active purpose
// in the tenant, with the subject's own current decision and preferences overlaid where one exists.
func (s *Service) GetPreferenceCenter(ctx context.Context, verificationID uuid.UUID, code string, id Identifier, lang string) (PreferenceCenter, error) {
	if err := s.verifyPreferenceIdentity(ctx, verificationID, id, code); err != nil {
		return PreferenceCenter{}, err
	}
	if lang != "en" {
		lang = "th"
	}
	pick := func(t Text) string {
		if lang == "en" && t.En != "" {
			return t.En
		}
		return t.Th
	}
	purposes, _, err := s.activePreferencePurposes(ctx)
	if err != nil {
		return PreferenceCenter{}, err
	}
	var ids []uuid.UUID
	for _, p := range purposes {
		if p.Status == "active" && p.CurrentVersion != nil {
			ids = append(ids, p.ID)
		}
	}
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	prefRows, err := q.ListPurposePreferences(ctx, ids)
	if err != nil {
		return PreferenceCenter{}, err
	}

	kind := crypto.IdentifierKind(id.Type)
	v, err := crypto.Normalize(kind, id.Value)
	if err != nil {
		return PreferenceCenter{}, invalid("identifier")
	}
	idx, err := s.Keyring.BlindIndex(ctx, kind, v)
	if err != nil {
		return PreferenceCenter{}, err
	}
	subjectID, err := q.FindSubjectByIdentifier(ctx, consentstore.FindSubjectByIdentifierParams{IdentifierType: string(kind), BlindIndex: idx})
	found := true
	if errors.Is(err, pgx.ErrNoRows) {
		found = false
	} else if err != nil {
		return PreferenceCenter{}, err
	}

	statuses := map[uuid.UUID]consentstore.ListSubjectStatusRow{}
	out := PreferenceCenter{Identifiers: []MaskedIdentifier{}, Purposes: []PreferenceItem{}}
	if found {
		sts, err := q.ListSubjectStatus(ctx, subjectID)
		if err != nil {
			return PreferenceCenter{}, err
		}
		for _, st := range sts {
			statuses[st.PurposeID] = st
		}
		masked, err := s.maskedIdentifiers(ctx, []uuid.UUID{subjectID})
		if err != nil {
			return PreferenceCenter{}, err
		}
		if m := masked[subjectID]; m != nil {
			out.Identifiers = m
		}
	}

	for _, p := range purposes {
		if p.Status != "active" || p.CurrentVersion == nil {
			continue
		}
		item := PreferenceItem{Code: p.Code, VersionNo: p.CurrentVersion.No, Name: pick(p.Live.Name), Description: pick(p.Live.Description),
			Text: pick(p.Live.ConsentText), ExplicitText: pick(p.Live.ExplicitText), IsSensitive: p.IsSensitive, Preferences: []PublicPreference{}}
		for _, row := range prefRows {
			if row.PurposeID != p.ID {
				continue
			}
			pref := toPreference(row)
			pp := PublicPreference{Code: pref.Code, Name: pick(pref.Name), Type: pref.Type}
			for _, o := range pref.Options {
				pp.Options = append(pp.Options, PublicOption{Value: o.Value, Label: pick(o.Label)})
			}
			item.Preferences = append(item.Preferences, pp)
		}
		if st, ok := statuses[p.ID]; ok {
			item.Decision = st.Status
			item.Current = json.RawMessage(st.Preferences)
			if st.ExpiresAt.Valid {
				t := st.ExpiresAt.Time
				item.ExpiresAt = &t
			}
		}
		out.Purposes = append(out.Purposes, item)
	}
	return out, nil
}

// channelForPreferenceUpdate attributes the preference-centre receipt to a real, active collection point —
// one already showing the first decided purpose when there is one, else any active point of the tenant —
// since consent.consent_receipts.collection_point_id is a hard NOT NULL FK and the preference centre itself
// isn't modelled as a collection point. A disclosed limitation (CLAUDE.md's own "minimal slice" pattern): the
// receipt's channel/collection_point then names wherever that purpose happens to also be collected, not that
// the subject actually used that specific form — add a dedicated collection-point-less receipt shape only if
// a future feature needs the distinction.
func (s *Service) channelForPreferenceUpdate(ctx context.Context, decisions []checkedDecision) (CollectionPoint, error) {
	cps, err := s.ListCollectionPoints(ctx)
	if err != nil {
		return CollectionPoint{}, err
	}
	var active []CollectionPoint
	for _, cp := range cps {
		if cp.Status == "active" {
			active = append(active, cp)
		}
	}
	if len(active) == 0 {
		return CollectionPoint{}, invalid("no active collection point to attribute this change to")
	}
	if len(decisions) > 0 {
		purposeID := decisions[0].purpose.PurposeID
		for _, cp := range active {
			if slices.ContainsFunc(cp.Purposes, func(p CPPurpose) bool { return p.PurposeID == purposeID }) {
				return cp, nil
			}
		}
	}
	return active[0], nil
}

// UpdatePreferences is the preference centre's own write path (CON-18, CON-13's self-service half): once OTP
// verification passes, any of the tenant's active purposes may be granted, refused or withdrawn in one call —
// unlike a public collection-point submission, withdrawal is allowed and nothing is required to be decided.
func (s *Service) UpdatePreferences(ctx context.Context, verificationID uuid.UUID, code string, id Identifier, decisions []Decision, language string) (Receipt, error) {
	if err := s.verifyPreferenceIdentity(ctx, verificationID, id, code); err != nil {
		return Receipt{}, err
	}
	if language != "en" {
		language = "th"
	}
	_, cpPurposes, err := s.activePreferencePurposes(ctx)
	if err != nil {
		return Receipt{}, err
	}
	synthetic := CollectionPoint{Purposes: cpPurposes}
	sub := Submission{Identifiers: []Identifier{id}, Decisions: decisions, Language: language, Source: "web"}
	kept, err := s.checkDecisions(ctx, synthetic, sub)
	if err != nil {
		return Receipt{}, err
	}
	channel, err := s.channelForPreferenceUpdate(ctx, kept)
	if err != nil {
		return Receipt{}, err
	}
	return s.writeDecisions(ctx, channel, sub, kept)
}
