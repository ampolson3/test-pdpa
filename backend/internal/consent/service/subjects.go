package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	consentstore "pdpa-platform/internal/consent/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/tenants"
)

// MaskedIdentifier is an identifier as the admin app shows it (rule 3: masked by default).
type MaskedIdentifier struct {
	Type     string
	Masked   string
	Primary  bool
	Verified bool
}

// SubjectSummary is a row of the subject list.
type SubjectSummary struct {
	ID           uuid.UUID
	Key          string
	Identifiers  []MaskedIdentifier
	LastActivity *time.Time
	CreatedAt    time.Time
}

// PurposeStatus is a subject's current state on one purpose.
type PurposeStatus struct {
	PurposeID        uuid.UUID
	PurposeCode      string
	PurposeName      Text
	IsSensitive      bool
	Status           string
	VersionNo        int32
	CurrentVersionNo *int32
	// NeedsReconsent: the consent was given to a version older than a published material change that asks for
	// re-consent (CON-12) — ask again before relying on it.
	NeedsReconsent bool
	Preferences    json.RawMessage
	ExpiresAt      *time.Time
	UpdatedAt      time.Time
}

// HistoryItem is one transaction of the subject's history, newest first.
type HistoryItem struct {
	ID                  uuid.UUID
	OccurredAt          time.Time
	Type                string
	PurposeCode         string
	PurposeName         Text
	VersionNo           int32
	Preferences         json.RawMessage
	ReasonCode          string
	ExpiresAt           *time.Time
	Source              string
	ReceiptNo           string
	Channel             string
	CollectionPointName string
	CapturedByName      string
}

// Profile is the data subject profile (CON-15 frontend).
type Profile struct {
	SubjectSummary
	Statuses []PurposeStatus
	History  []HistoryItem
}

// Subjects lists the most recently active data subjects, or — with an identifier — the one it belongs to
// (exact match through the blind index; nothing is searched in clear text).
func (s *Service) Subjects(ctx context.Context, identifierType, value string) ([]SubjectSummary, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	var ids []uuid.UUID
	var rows []consentstore.RecentSubjectsRow
	if strings.TrimSpace(value) != "" {
		kind := crypto.IdentifierKind(identifierType)
		idx, err := s.Keyring.BlindIndex(ctx, kind, value)
		if errors.Is(err, crypto.ErrInvalidIdentifier) {
			return []SubjectSummary{}, nil
		}
		if err != nil {
			return nil, err
		}
		id, err := q.FindSubjectByIdentifier(ctx, consentstore.FindSubjectByIdentifierParams{IdentifierType: string(kind), BlindIndex: idx})
		if errors.Is(err, pgx.ErrNoRows) {
			return []SubjectSummary{}, nil
		}
		if err != nil {
			return nil, err
		}
		r, err := q.GetSubject(ctx, id)
		if err != nil {
			return nil, err
		}
		rows = []consentstore.RecentSubjectsRow{{ID: r.ID, SubjectKey: r.SubjectKey, LastActivityAt: r.LastActivityAt, CreatedAt: r.CreatedAt}}
	} else {
		var err error
		if rows, err = q.RecentSubjects(ctx, 100); err != nil {
			return nil, err
		}
	}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	masked, err := s.maskedIdentifiers(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]SubjectSummary, 0, len(rows))
	for _, r := range rows {
		sm := SubjectSummary{ID: r.ID, Key: r.SubjectKey, Identifiers: masked[r.ID], CreatedAt: r.CreatedAt.Time}
		if r.LastActivityAt.Valid {
			t := r.LastActivityAt.Time
			sm.LastActivity = &t
		}
		if sm.Identifiers == nil {
			sm.Identifiers = []MaskedIdentifier{}
		}
		out = append(out, sm)
	}
	return out, nil
}

func (s *Service) maskedIdentifiers(ctx context.Context, subjects []uuid.UUID) (map[uuid.UUID][]MaskedIdentifier, error) {
	out := map[uuid.UUID][]MaskedIdentifier{}
	if len(subjects) == 0 {
		return out, nil
	}
	rows, err := consentstore.New(pdb.MustTxFromContext(ctx)).ListIdentifiers(ctx, subjects)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		plain, err := s.Keyring.Decrypt(ctx, cryptoClass, identifierContext, r.ValueEnc)
		if err != nil {
			return nil, err
		}
		out[r.SubjectID] = append(out[r.SubjectID], MaskedIdentifier{Type: r.IdentifierType, Masked: mask(r.IdentifierType, string(plain)), Primary: r.IsPrimary, Verified: r.VerifiedAt.Valid})
	}
	return out, nil
}

func mask(kind, v string) string {
	switch kind {
	case "email", "phone":
		return notify.Mask(v)
	}
	r := []rune(v)
	if len(r) <= 4 {
		return strings.Repeat("*", len(r))
	}
	return strings.Repeat("*", len(r)-4) + string(r[len(r)-4:])
}

// GetProfile returns a subject's statuses and history.
func (s *Service) GetProfile(ctx context.Context, id uuid.UUID) (Profile, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.GetSubject(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	masked, err := s.maskedIdentifiers(ctx, []uuid.UUID{id})
	if err != nil {
		return Profile{}, err
	}
	p := Profile{SubjectSummary: SubjectSummary{ID: r.ID, Key: r.SubjectKey, Identifiers: masked[id], CreatedAt: r.CreatedAt.Time}}
	if p.Identifiers == nil {
		p.Identifiers = []MaskedIdentifier{}
	}
	if r.LastActivityAt.Valid {
		t := r.LastActivityAt.Time
		p.LastActivity = &t
	}
	sts, err := q.ListSubjectStatus(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	for _, st := range sts {
		ps := PurposeStatus{PurposeID: st.PurposeID, PurposeCode: st.Code, PurposeName: Text{Th: st.NameTh, En: deref(st.NameEn)}, IsSensitive: st.IsSensitive,
			Status: st.Status, VersionNo: st.VersionNo, CurrentVersionNo: st.CurrentVersionNo, Preferences: json.RawMessage(st.Preferences), UpdatedAt: st.UpdatedAt.Time}
		if st.ExpiresAt.Valid {
			t := st.ExpiresAt.Time
			ps.ExpiresAt = &t
		}
		ps.NeedsReconsent = st.Status == StatusActive && st.CurrentVersionNo != nil && *st.CurrentVersionNo > st.VersionNo &&
			st.CurrentRequiresReconsent != nil && *st.CurrentRequiresReconsent
		p.Statuses = append(p.Statuses, ps)
	}
	if p.Statuses == nil {
		p.Statuses = []PurposeStatus{}
	}
	txs, err := q.ListSubjectTransactions(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	var users []uuid.UUID
	for _, t := range txs {
		if t.CapturedByUserID.Valid {
			users = append(users, uuid.UUID(t.CapturedByUserID.Bytes))
		}
	}
	names, err := iamservice.AllNames(ctx, users)
	if err != nil {
		return Profile{}, err
	}
	p.History = make([]HistoryItem, 0, len(txs))
	for _, t := range txs {
		h := HistoryItem{ID: t.ID, OccurredAt: t.OccurredAt.Time, Type: t.TransactionType, PurposeCode: t.PurposeCode, PurposeName: Text{Th: t.PurposeNameTh, En: deref(t.PurposeNameEn)},
			VersionNo: t.VersionNo, Preferences: json.RawMessage(t.Preferences), ReasonCode: deref(t.ReasonCode), Source: t.Source, ReceiptNo: t.ReceiptNo,
			Channel: t.Channel, CollectionPointName: t.CollectionPointName}
		if t.ExpiresAt.Valid {
			e := t.ExpiresAt.Time
			h.ExpiresAt = &e
		}
		if t.CapturedByUserID.Valid {
			h.CapturedByName = names[uuid.UUID(t.CapturedByUserID.Bytes)]
		}
		p.History = append(p.History, h)
	}
	return p, nil
}

// VerifyResult is the outcome of replaying a subject's receipt chain.
type VerifyResult struct {
	OK       bool
	Checked  int
	BrokenAt string // receipt number of the first receipt that fails
	Reason   string // hash_mismatch | prev_hash_mismatch
}

// VerifySubject recomputes every receipt of a subject from its stored columns and transactions and checks each
// links to the one before — an edit, deletion or insertion anywhere shows up (CON-15 acceptance).
func (s *Service) VerifySubject(ctx context.Context, id uuid.UUID) (VerifyResult, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	if _, err := q.GetSubject(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return VerifyResult{}, ErrNotFound
	} else if err != nil {
		return VerifyResult{}, err
	}
	rs, err := q.ListSubjectReceipts(ctx, id)
	if err != nil {
		return VerifyResult{}, err
	}
	var rids []uuid.UUID
	for _, r := range rs {
		rids = append(rids, r.ID)
	}
	txs, err := q.ListReceiptTransactions(ctx, rids)
	if err != nil {
		return VerifyResult{}, err
	}
	res := VerifyResult{OK: true}
	prev := ""
	for _, r := range rs {
		res.Checked++
		if r.PrevHash != prev {
			return VerifyResult{Checked: res.Checked, BrokenAt: r.ReceiptNo, Reason: "prev_hash_mismatch"}, nil
		}
		rec := receiptRecord{ReceiptNo: r.ReceiptNo, SubjectID: r.SubjectID, CollectionPointID: r.CollectionPointID, Channel: r.Channel,
			CapturedBy: uuidPtr(r.CapturedByUserID), UserAgent: deref(r.UserAgent), Language: deref(r.Language), OccurredAt: r.OccurredAt.Time, PrevHash: r.PrevHash}
		if r.Ip != nil {
			rec.IP = r.Ip.String()
		}
		for _, t := range txs {
			if t.ReceiptID != r.ID {
				continue
			}
			tr := txRecord{ID: t.ID, PurposeID: t.PurposeID, PurposeVersionID: t.PurposeVersionID, Type: t.TransactionType, Preferences: json.RawMessage(t.Preferences),
				ReasonCode: deref(t.ReasonCode), Source: t.Source}
			if t.ExpiresAt.Valid {
				e := t.ExpiresAt.Time
				tr.ExpiresAt = &e
			}
			if !t.OccurredAt.Time.Equal(r.OccurredAt.Time) {
				return VerifyResult{Checked: res.Checked, BrokenAt: r.ReceiptNo, Reason: "hash_mismatch"}, nil
			}
			rec.Transactions = append(rec.Transactions, tr)
		}
		if rec.hash() != r.Hash {
			return VerifyResult{Checked: res.Checked, BrokenAt: r.ReceiptNo, Reason: "hash_mismatch"}, nil
		}
		prev = r.Hash
	}
	return res, nil
}

// PublicPurpose is a purpose as a consent form shows it, in one language.
type PublicPurpose struct {
	Code         string
	VersionNo    int32
	Name         string
	Description  string
	Text         string
	ExplicitText string
	IsSensitive  bool
	Required     bool
	MinAge       *int16
	Preferences  []PublicPreference
}

// PublicPreference is a preference with labels in one language.
type PublicPreference struct {
	Code    string
	Name    string
	Type    string
	Options []PublicOption
}

// PublicOption is one preference choice.
type PublicOption struct{ Value, Label string }

// PublicCollectionPoint is GET /public/v1/collection-points/{key} (BP-01 t1–t2).
type PublicCollectionPoint struct {
	Code     string
	Name     string
	Channel  string
	Language string
	Purposes []PublicPurpose
}

// PublicView returns an active collection point for a consent form (tenant from its public key).
func (s *Service) PublicView(ctx context.Context, id uuid.UUID, lang string) (PublicCollectionPoint, error) {
	cp, err := s.GetCollectionPoint(ctx, id)
	if err != nil {
		return PublicCollectionPoint{}, err
	}
	if cp.Status != "active" {
		return PublicCollectionPoint{}, ErrNotFound
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
	var ids []uuid.UUID
	for _, p := range cp.Purposes {
		ids = append(ids, p.PurposeID)
	}
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	prefs, err := q.ListPurposePreferences(ctx, ids)
	if err != nil {
		return PublicCollectionPoint{}, err
	}
	out := PublicCollectionPoint{Code: cp.Code, Name: cp.Name, Channel: cp.Channel, Language: lang, Purposes: []PublicPurpose{}}
	for _, p := range cp.Purposes {
		if p.Status != "active" || p.CurrentVersionNo == nil {
			continue
		}
		pr, err := q.GetPurpose(ctx, p.PurposeID)
		if err != nil {
			return PublicCollectionPoint{}, err
		}
		pp := PublicPurpose{Code: p.Code, VersionNo: *p.CurrentVersionNo, Name: pick(p.Name), Description: pick(Text{Th: deref(pr.DescriptionTh), En: deref(pr.DescriptionEn)}),
			Text: pick(p.ConsentText), ExplicitText: pick(p.ExplicitText), IsSensitive: p.IsSensitive, Required: p.Required, MinAge: p.MinAge, Preferences: []PublicPreference{}}
		for _, row := range prefs {
			if row.PurposeID != p.PurposeID {
				continue
			}
			pref := toPreference(row)
			ppref := PublicPreference{Code: pref.Code, Name: pick(pref.Name), Type: pref.Type}
			for _, o := range pref.Options {
				ppref.Options = append(ppref.Options, PublicOption{Value: o.Value, Label: pick(o.Label)})
			}
			pp.Preferences = append(pp.Preferences, ppref)
		}
		out.Purposes = append(out.Purposes, pp)
	}
	return out, nil
}

// Settings is CON-17: where consent data is stored and how identifiers are protected.
type Settings struct {
	DataRegion    string
	KeyManagement string // openbao_transit | local_development
}

// GetSettings reports the tenant's data region and the key management in use.
func (s *Service) GetSettings(ctx context.Context) (Settings, error) {
	region, err := tenants.Region(ctx)
	if err != nil {
		return Settings{}, err
	}
	km := "local_development"
	if _, ok := s.Keyring.KEK.(*crypto.Transit); ok {
		km = "openbao_transit"
	}
	return Settings{DataRegion: region, KeyManagement: km}, nil
}
