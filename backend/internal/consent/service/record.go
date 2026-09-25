package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	consentstore "pdpa-platform/internal/consent/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/i18n"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/notify"
)

const (
	cryptoClass       = "consent"
	identifierContext = "consent.subject_identifiers.value_enc"
)

// WithdrawalReasons are the reasons a withdrawal can record (CON-13); the UI localizes them.
var WithdrawalReasons = []string{"no_longer_interested", "too_many_messages", "privacy_concern", "service_ended", "other"}

// Identifier names a data subject (e-mail, phone, national id, customer id …). Values are never logged.
type Identifier struct {
	Type  string
	Value string
}

// Decision is the subject's choice on one purpose, for the version they were shown.
type Decision struct {
	PurposeCode      string
	PurposeVersionNo int32
	Decision         string // CONSENTED | NOT_CONSENTED | WITHDRAWN
	Preferences      map[string]any
	ReasonCode       string // why a consent is withdrawn (optional)
}

// Submission is one act of the data subject (or staff on their behalf): one receipt, one transaction per decision.
type Submission struct {
	CollectionPointID uuid.UUID
	SubjectID         *uuid.UUID // an existing subject (staff on the profile page); otherwise Identifiers find or create one
	Identifiers       []Identifier
	Decisions         []Decision
	Language          string
	Source            string // web | app | api | staff
	IP                *netip.Addr
	UserAgent         string
	CapturedBy        *uuid.UUID
	IdempotencyKey    string
	// Public submissions (a web form) decide every purpose of the form and can't withdraw — that needs the
	// verified preference centre (CON-18/19).
	Public bool
}

// RecordedTransaction is one transaction written by Record.
type RecordedTransaction struct {
	ID          uuid.UUID
	PurposeCode string
	Type        string
	Status      string
}

// Receipt is what Record returns.
type Receipt struct {
	ID           uuid.UUID
	No           string
	SubjectID    uuid.UUID
	OccurredAt   time.Time
	Transactions []RecordedTransaction
}

// DecisionError names the purposes a submission got wrong (codes the caller maps to 422 field errors).
type DecisionError struct {
	Fields []FieldError
}

// FieldError is one bad decision or identifier.
type FieldError struct {
	Field string // purpose code, or "identifiers"
	Code  string // unknown_purpose | stale_version | required | missing | duplicate | invalid_preference | withdraw_not_allowed | invalid
}

func (e *DecisionError) Error() string {
	return fmt.Sprintf("consent: %d invalid decision(s)", len(e.Fields))
}
func (e *DecisionError) Unwrap() error { return ErrInvalidRequest }

// Record writes a submission: finds or creates the data subject by blind index, appends a receipt to the subject's
// hash chain and one append-only transaction per decision, moves the status projection along ST-01, and publishes
// consent.* events through the outbox — all in the caller's transaction (SEQ-04 steps 5–10).
func (s *Service) Record(ctx context.Context, sub Submission) (Receipt, error) {
	cp, err := s.GetCollectionPoint(ctx, sub.CollectionPointID)
	if err != nil {
		return Receipt{}, err
	}
	if cp.Status != "active" {
		return Receipt{}, ErrNotFound
	}
	if sub.Language != "en" {
		sub.Language = "th"
	}
	decisions, err := s.checkDecisions(ctx, cp, sub)
	if err != nil {
		return Receipt{}, err
	}
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	subjectID, emails, err := s.resolveSubject(ctx, sub)
	if err != nil {
		return Receipt{}, err
	}
	if err := q.LockSubjectChain(ctx, subjectID); err != nil {
		return Receipt{}, err
	}
	prev := ""
	occurred := s.now().Truncate(time.Microsecond)
	last, err := q.LastReceipt(ctx, subjectID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return Receipt{}, err
	default:
		prev = last.Hash
		if !occurred.After(last.OccurredAt.Time) {
			occurred = last.OccurredAt.Time.Add(time.Microsecond)
		}
	}
	rid, err := uuid.NewV7()
	if err != nil {
		return Receipt{}, err
	}
	rec := receiptRecord{ReceiptNo: newReceiptNo(occurred), SubjectID: subjectID, CollectionPointID: cp.ID, Channel: cp.Channel, CapturedBy: sub.CapturedBy,
		UserAgent: truncate(sub.UserAgent, 500), Language: sub.Language, OccurredAt: occurred, PrevHash: prev}
	if sub.IP != nil {
		rec.IP = sub.IP.String()
	}
	type statusWrite struct {
		purpose   uuid.UUID
		status    string
		version   uuid.UUID
		prefs     []byte
		expiresAt pgtype.Timestamptz
		txID      uuid.UUID
	}
	var writes []statusWrite
	out := Receipt{ID: rid, No: rec.ReceiptNo, SubjectID: subjectID, OccurredAt: occurred}
	for _, d := range decisions {
		cur, err := q.LockStatus(ctx, consentstore.LockStatusParams{SubjectID: subjectID, PurposeID: d.purpose.PurposeID})
		current, curPrefs, curExpires := "", []byte(nil), pgtype.Timestamptz{}
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return Receipt{}, err
		default:
			current, curPrefs, curExpires = cur.Status, cur.Preferences, cur.ExpiresAt
		}
		prefs := canonical(d.in.Preferences)
		txType, status, err := Decide(current, d.in.Decision, current == StatusActive && d.in.Decision == TxConsented && !sameJSON(curPrefs, prefs))
		if err != nil {
			return Receipt{}, err
		}
		var expires *time.Time
		switch txType {
		case TxConsented, TxExtended:
			if d.purpose.LifespanDays != nil {
				e := occurred.AddDate(0, 0, int(*d.purpose.LifespanDays))
				expires = &e
			}
		case TxChangedPreferences:
			if curExpires.Valid {
				e := curExpires.Time
				expires = &e
			}
		}
		if txType == TxWithdrawn || txType == TxNotConsented {
			prefs = nil
		}
		tid, err := uuid.NewV7()
		if err != nil {
			return Receipt{}, err
		}
		rec.Transactions = append(rec.Transactions, txRecord{ID: tid, PurposeID: d.purpose.PurposeID, PurposeVersionID: *d.purpose.CurrentVersionID, Type: txType,
			Preferences: json.RawMessage(prefs), ReasonCode: d.in.ReasonCode, ExpiresAt: expires, Source: sub.Source})
		writes = append(writes, statusWrite{purpose: d.purpose.PurposeID, status: status, version: *d.purpose.CurrentVersionID, prefs: prefs, expiresAt: tsPtr(expires), txID: tid})
		out.Transactions = append(out.Transactions, RecordedTransaction{ID: tid, PurposeCode: d.purpose.Code, Type: txType, Status: status})
	}
	rec.Hash = rec.hash()
	var ip *netip.Addr
	if sub.IP != nil {
		a := sub.IP.Unmap()
		ip = &a
	}
	if err := q.InsertReceipt(ctx, consentstore.InsertReceiptParams{ID: rid, ReceiptNo: rec.ReceiptNo, SubjectID: subjectID, CollectionPointID: cp.ID, Channel: cp.Channel,
		CapturedByUserID: pgUUID(sub.CapturedBy), Ip: ip, UserAgent: optText(rec.UserAgent), Language: optText(sub.Language),
		OccurredAt: pgtype.Timestamptz{Time: occurred, Valid: true}, PrevHash: prev, Hash: rec.Hash}); err != nil {
		return Receipt{}, err
	}
	for _, t := range rec.Transactions {
		if err := q.InsertTransaction(ctx, consentstore.InsertTransactionParams{ID: t.ID, OccurredAt: pgtype.Timestamptz{Time: occurred, Valid: true}, ReceiptID: rid,
			SubjectID: subjectID, PurposeID: t.PurposeID, PurposeVersionID: t.PurposeVersionID, TransactionType: t.Type, Preferences: t.Preferences,
			ReasonCode: optText(t.ReasonCode), ExpiresAt: tsPtr(t.ExpiresAt), Source: t.Source, IdempotencyKey: optText(truncate(sub.IdempotencyKey, 80))}); err != nil {
			return Receipt{}, err
		}
	}
	for _, w := range writes {
		if err := q.UpsertStatus(ctx, consentstore.UpsertStatusParams{SubjectID: subjectID, PurposeID: w.purpose, Status: w.status, PurposeVersionID: w.version,
			LastTransactionID: w.txID, Preferences: w.prefs, ExpiresAt: w.expiresAt}); err != nil {
			return Receipt{}, err
		}
	}
	if err := q.TouchSubject(ctx, subjectID); err != nil {
		return Receipt{}, err
	}
	for i, t := range out.Transactions {
		ev := eventFor(t.Type)
		if ev == "" || s.Events == nil {
			continue
		}
		if _, err := s.Events.Publish(ctx, events.Event{Type: ev, AggregateType: SubjectType, AggregateID: subjectID, OccurredAt: occurred,
			Data: map[string]any{"subject_ref": subjectID.String(), "purpose_code": t.PurposeCode, "purpose_version": *decisions[i].purpose.CurrentVersionNo,
				"channel": cp.Channel, "occurred_at": occurred.Format(time.RFC3339Nano)}}); err != nil {
			return Receipt{}, err
		}
	}
	if sub.Source == "staff" {
		if err := s.audit(ctx, "consent.record.on_behalf", SubjectType, subjectID, nil, map[string]any{"receipt_no": rec.ReceiptNo, "collection_point": cp.Code, "decisions": len(out.Transactions)}); err != nil {
			return Receipt{}, err
		}
	}
	if err := s.sendReceipt(ctx, emails, sub.Language, out, decisions); err != nil {
		return Receipt{}, err
	}
	return out, nil
}

type checkedDecision struct {
	in      Decision
	purpose CPPurpose
}

func (s *Service) checkDecisions(ctx context.Context, cp CollectionPoint, sub Submission) ([]checkedDecision, error) {
	var errs []FieldError
	var out []checkedDecision
	seen := map[string]bool{}
	var ids []uuid.UUID
	for _, p := range cp.Purposes {
		ids = append(ids, p.PurposeID)
	}
	prefRows, err := consentstore.New(pdb.MustTxFromContext(ctx)).ListPurposePreferences(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(sub.Decisions) == 0 {
		errs = append(errs, FieldError{"decisions", "missing"})
	}
	for _, d := range sub.Decisions {
		i := slices.IndexFunc(cp.Purposes, func(p CPPurpose) bool { return p.Code == d.PurposeCode })
		switch {
		case i < 0:
			errs = append(errs, FieldError{d.PurposeCode, "unknown_purpose"})
			continue
		case seen[d.PurposeCode]:
			errs = append(errs, FieldError{d.PurposeCode, "duplicate"})
			continue
		}
		seen[d.PurposeCode] = true
		p := cp.Purposes[i]
		switch {
		case p.Status != "active" || p.CurrentVersionNo == nil:
			errs = append(errs, FieldError{d.PurposeCode, "unknown_purpose"})
		case d.PurposeVersionNo != *p.CurrentVersionNo:
			errs = append(errs, FieldError{d.PurposeCode, "stale_version"})
		case sub.Public && d.Decision == TxWithdrawn:
			errs = append(errs, FieldError{d.PurposeCode, "withdraw_not_allowed"})
		case p.Required && d.Decision == TxNotConsented:
			errs = append(errs, FieldError{d.PurposeCode, "required"})
		case len(d.Preferences) > 0 && (d.Decision != TxConsented || !validPreferences(prefRows, p.PurposeID, d.Preferences)):
			errs = append(errs, FieldError{d.PurposeCode, "invalid_preference"})
		case d.Decision != TxConsented && d.Decision != TxNotConsented && d.Decision != TxWithdrawn:
			errs = append(errs, FieldError{d.PurposeCode, "invalid"})
		case d.ReasonCode != "" && (d.Decision == TxConsented || !slices.Contains(WithdrawalReasons, d.ReasonCode)):
			errs = append(errs, FieldError{d.PurposeCode, "invalid_reason"})
		default:
			out = append(out, checkedDecision{in: d, purpose: p})
		}
	}
	if sub.Public {
		for _, p := range cp.Purposes {
			if !seen[p.Code] {
				errs = append(errs, FieldError{p.Code, "missing"}) // a web form decides every purpose it shows
			}
		}
	}
	if len(errs) > 0 {
		return nil, &DecisionError{Fields: errs}
	}
	return out, nil
}

func validPreferences(rows []consentstore.ListPurposePreferencesRow, purpose uuid.UUID, prefs map[string]any) bool {
	for code, v := range prefs {
		i := slices.IndexFunc(rows, func(r consentstore.ListPurposePreferencesRow) bool { return r.PurposeID == purpose && r.Code == code })
		if i < 0 {
			return false
		}
		var opts []PreferenceOption
		_ = json.Unmarshal(rows[i].Options, &opts)
		ok := func(s any) bool {
			str, isStr := s.(string)
			return isStr && slices.ContainsFunc(opts, func(o PreferenceOption) bool { return o.Value == str })
		}
		switch x := v.(type) {
		case []any:
			for _, e := range x {
				if !ok(e) {
					return false
				}
			}
		default:
			if !ok(x) {
				return false
			}
		}
	}
	return true
}

// resolveSubject finds the data subject every given identifier points to, creating it (and adding identifiers it
// lacks) as needed. Identifiers of two different subjects in one submission are refused.
func (s *Service) resolveSubject(ctx context.Context, sub Submission) (uuid.UUID, []string, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	type norm struct {
		kind  string
		value string
		index []byte
	}
	var ids []norm
	var emails []string
	for _, id := range sub.Identifiers {
		kind := crypto.IdentifierKind(id.Type)
		if !slices.Contains([]crypto.IdentifierKind{crypto.KindEmail, crypto.KindPhone, crypto.KindNationalID, crypto.KindCustomerID, crypto.KindPassport, crypto.KindLineUID, crypto.KindOther}, kind) {
			return uuid.Nil, nil, &DecisionError{Fields: []FieldError{{"identifiers", "invalid"}}}
		}
		v, err := crypto.Normalize(kind, id.Value)
		if err != nil || len(v) > 320 {
			return uuid.Nil, nil, &DecisionError{Fields: []FieldError{{"identifiers", "invalid"}}}
		}
		idx, err := s.Keyring.BlindIndex(ctx, kind, v)
		if err != nil {
			return uuid.Nil, nil, err
		}
		if slices.ContainsFunc(ids, func(n norm) bool { return n.kind == string(kind) && n.value == v }) {
			continue
		}
		ids = append(ids, norm{string(kind), v, idx})
		if kind == crypto.KindEmail {
			emails = append(emails, v)
		}
	}
	var subject uuid.UUID
	if sub.SubjectID != nil {
		if _, err := q.GetSubject(ctx, *sub.SubjectID); err != nil {
			return uuid.Nil, nil, ErrNotFound
		}
		subject = *sub.SubjectID
	} else if len(ids) == 0 {
		return uuid.Nil, nil, &DecisionError{Fields: []FieldError{{"identifiers", "missing"}}}
	}
	var missing []norm
	for _, n := range ids {
		found, err := q.FindSubjectByIdentifier(ctx, consentstore.FindSubjectByIdentifierParams{IdentifierType: n.kind, BlindIndex: n.index})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			missing = append(missing, n)
		case err != nil:
			return uuid.Nil, nil, err
		case subject == uuid.Nil:
			subject = found
		case subject != found:
			return uuid.Nil, nil, ErrSubjectConflict
		}
	}
	if subject == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return uuid.Nil, nil, err
		}
		if err := q.InsertSubject(ctx, consentstore.InsertSubjectParams{ID: id, SubjectKey: "DS-" + randomCode(12)}); err != nil {
			return uuid.Nil, nil, err
		}
		subject = id
	}
	existing, err := q.ListIdentifiers(ctx, []uuid.UUID{subject})
	if err != nil {
		return uuid.Nil, nil, err
	}
	primary := len(existing) == 0
	for _, n := range missing {
		enc, err := s.Keyring.Encrypt(ctx, cryptoClass, identifierContext, []byte(n.value))
		if err != nil {
			return uuid.Nil, nil, err
		}
		if err := q.InsertIdentifier(ctx, consentstore.InsertIdentifierParams{SubjectID: subject, IdentifierType: n.kind, ValueEnc: enc, BlindIndex: n.index, IsPrimary: primary}); err != nil {
			return uuid.Nil, nil, err
		}
		primary = false
	}
	return subject, emails, nil
}

// sendReceipt e-mails the receipt to the addresses the subject just gave (CON-15), in their language.
func (s *Service) sendReceipt(ctx context.Context, emails []string, lang string, r Receipt, decisions []checkedDecision) error {
	if s.Notify == nil || len(emails) == 0 {
		return nil
	}
	l := i18n.Lang(lang)
	var lines []string
	for i, t := range r.Transactions {
		name := decisions[i].purpose.Name.Th
		if l == i18n.En && decisions[i].purpose.Name.En != "" {
			name = decisions[i].purpose.Name.En
		}
		lines = append(lines, "• "+name+": "+i18n.Message("consent.decision."+t.Type, l))
	}
	loc, _ := time.LoadLocation("Asia/Bangkok")
	for _, e := range emails[:1] {
		id := r.SubjectID
		if _, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "consent.receipt", Channel: notify.ChannelEmail, Language: lang, RecipientAddress: e,
			Vars:       map[string]any{"receipt_no": r.No, "occurred_at": r.OccurredAt.In(loc).Format("2006-01-02 15:04"), "decisions": strings.Join(lines, "\n")},
			EntityType: SubjectType, EntityID: &id, Urgent: true}); err != nil {
			return err
		}
	}
	return nil
}

// ---- receipt hash (tamper evidence: a receipt links to the subject's previous one) ----

type txRecord struct {
	ID               uuid.UUID       `json:"id"`
	PurposeID        uuid.UUID       `json:"purpose_id"`
	PurposeVersionID uuid.UUID       `json:"purpose_version_id"`
	Type             string          `json:"type"`
	Preferences      json.RawMessage `json:"preferences,omitempty"`
	ReasonCode       string          `json:"reason_code,omitempty"`
	ExpiresAt        *time.Time      `json:"expires_at,omitempty"`
	Source           string          `json:"source"`
}

type receiptRecord struct {
	Tag               string     `json:"v"`
	ReceiptNo         string     `json:"receipt_no"`
	SubjectID         uuid.UUID  `json:"subject_id"`
	CollectionPointID uuid.UUID  `json:"collection_point_id"`
	Channel           string     `json:"channel"`
	CapturedBy        *uuid.UUID `json:"captured_by,omitempty"`
	IP                string     `json:"ip,omitempty"`
	UserAgent         string     `json:"user_agent,omitempty"`
	Language          string     `json:"language"`
	OccurredAt        time.Time  `json:"occurred_at"`
	PrevHash          string     `json:"prev_hash"`
	Transactions      []txRecord `json:"transactions"`
	Hash              string     `json:"-"`
}

// hash is SHA-256 over the canonical JSON of the receipt and its transactions (sorted by id), which includes the
// previous receipt's hash (SEQ-04: "receipt hash = SHA-256(prev_hash + canonical JSON)").
func (r receiptRecord) hash() string {
	r.Tag = "consent-receipt/v1"
	r.OccurredAt = r.OccurredAt.UTC()
	txs := slices.Clone(r.Transactions)
	for i := range txs {
		txs[i].Preferences = json.RawMessage(canonical(rawMap(txs[i].Preferences)))
		if txs[i].ExpiresAt != nil {
			e := txs[i].ExpiresAt.UTC()
			txs[i].ExpiresAt = &e
		}
	}
	slices.SortFunc(txs, func(a, b txRecord) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	r.Transactions = txs
	b, _ := json.Marshal(r)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// canonical is JSON with sorted keys (encoding/json sorts map keys); nil for an empty map.
func canonical(m map[string]any) []byte {
	if len(m) == 0 {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}

func rawMap(b json.RawMessage) map[string]any {
	if len(b) == 0 {
		return nil
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func sameJSON(a, b []byte) bool {
	return reflect.DeepEqual(rawMap(a), rawMap(b))
}

var b32 = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

func randomCode(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return b32.EncodeToString(buf)[:n]
}

func newReceiptNo(t time.Time) string {
	loc, _ := time.LoadLocation("Asia/Bangkok")
	return "CR-" + t.In(loc).Format("20060102") + "-" + randomCode(10)
}

func tsPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
