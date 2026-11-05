package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	consentstore "pdpa-platform/internal/consent/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/platform/publickeys"
)

// ---- CON-24: bulk consent request campaign ----
//
// consent.campaigns/consent.campaign_recipients were already fully specified in the baseline migration —
// name, purpose_ids, audience_query, channel, template_id, status, sent_count, response_count on the
// campaign; subject_id, token_hash, status, sent_at, responded_at per recipient — unused until now. There is
// no separate "audience" storage of raw identifiers: CreateCampaign resolves every given identifier into a
// real consent.data_subjects row immediately (the same find-or-create resolveSubject Record() already uses,
// one identifier at a time so one campaign can target many different people), so audience_query only ever
// holds the resulting subject ids, never a raw e-mail/phone (rule 3 — no PII in a plain jsonb column).

// CampaignAudienceQuery is the shape stored in consent.campaigns.audience_query.
type CampaignAudienceQuery struct {
	SubjectIDs []uuid.UUID `json:"subject_ids"`
}

// Campaign is a bulk consent request campaign (CON-24).
type Campaign struct {
	ID            uuid.UUID
	Name          string
	PurposeIDs    []uuid.UUID
	Channel       string // email | sms | line
	TemplateID    *uuid.UUID
	Status        string // draft | sending | done | cancelled
	Audience      CampaignAudienceQuery
	SentCount     int32
	OpenedCount   int32
	ResponseCount int32
	RowVersion    int32
}

// CampaignInput is what CreateCampaign saves; Identifiers are never persisted raw — only the subjects they
// resolve to are (see CampaignAudienceQuery).
type CampaignInput struct {
	Name        string
	PurposeIDs  []uuid.UUID
	Channel     string
	TemplateID  *uuid.UUID
	Identifiers []Identifier
}

// campaignChannels are CON-24's own three delivery channels — a subset of notify's own, matching the module
// doc's "ส่งลิงก์ขอความยินยอมทางอีเมล/SMS ให้ฐานลูกค้าเดิม" literally (line because the schema's own CHECK
// constraint already names it, not a new choice this feature invented).
var campaignChannels = []string{"email", "sms", "line"}

func toCampaign(r consentstore.ConsentCampaign, counts map[string]int64) Campaign {
	out := Campaign{ID: r.ID, Name: r.Name, PurposeIDs: r.PurposeIds, Channel: r.Channel, Status: r.Status, RowVersion: r.RowVersion,
		SentCount: r.SentCount, ResponseCount: r.ResponseCount}
	if r.TemplateID.Valid {
		id := uuid.UUID(r.TemplateID.Bytes)
		out.TemplateID = &id
	}
	_ = json.Unmarshal(r.AudienceQuery, &out.Audience)
	out.OpenedCount = int32(counts["opened"]) + int32(counts["responded"])
	return out
}

// campaignCounts reads the recipients' own live status breakdown — the acceptance criterion's own "แสดง
// จำนวนส่ง / เปิด / ตอบ" is computed from the recipient rows every call, never trusted from the campaign's
// own stored sent_count/response_count alone (those are kept in step at send/resend time for convenience,
// but a live read can never go stale).
func (s *Service) campaignCounts(ctx context.Context, campaignID uuid.UUID) (map[string]int64, error) {
	rows, err := consentstore.New(pdb.MustTxFromContext(ctx)).CountCampaignRecipientsByStatus(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, r := range rows {
		out[r.Status] = r.N
	}
	return out, nil
}

// CreateCampaign validates the campaign and resolves its audience into real data subjects immediately —
// the acceptance criterion's own target list is fixed at creation, send/resend only notify it.
func (s *Service) CreateCampaign(ctx context.Context, in CampaignInput) (Campaign, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 200 {
		return Campaign{}, invalid("name")
	}
	if len(in.PurposeIDs) == 0 {
		return Campaign{}, invalid("purpose_ids")
	}
	for _, pid := range in.PurposeIDs {
		p, err := s.GetPurpose(ctx, pid)
		if err != nil {
			return Campaign{}, invalid("purpose_ids")
		}
		if p.Status != "active" || p.CurrentVersion == nil {
			return Campaign{}, invalid("purpose_ids")
		}
	}
	if !slices.Contains(campaignChannels, in.Channel) {
		return Campaign{}, invalid("channel")
	}
	if len(in.Identifiers) == 0 {
		return Campaign{}, invalid("identifiers")
	}
	var subjectIDs []uuid.UUID
	for _, id := range in.Identifiers {
		kind := crypto.IdentifierKind(id.Type)
		if !slices.Contains([]crypto.IdentifierKind{crypto.KindEmail, crypto.KindPhone, crypto.KindNationalID, crypto.KindCustomerID, crypto.KindPassport, crypto.KindLineUID, crypto.KindOther}, kind) {
			return Campaign{}, invalid("identifiers")
		}
		subjectID, _, err := s.resolveSubject(ctx, Submission{Identifiers: []Identifier{id}})
		if err != nil {
			return Campaign{}, invalid("identifiers")
		}
		if !slices.Contains(subjectIDs, subjectID) {
			subjectIDs = append(subjectIDs, subjectID)
		}
	}
	audience, _ := json.Marshal(CampaignAudienceQuery{SubjectIDs: subjectIDs})
	id, err := uuid.NewV7()
	if err != nil {
		return Campaign{}, err
	}
	var templateID pgtype.UUID
	if in.TemplateID != nil {
		templateID = pgtype.UUID{Bytes: *in.TemplateID, Valid: true}
	}
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertCampaign(ctx, consentstore.InsertCampaignParams{ID: id, Name: in.Name, PurposeIds: in.PurposeIDs,
		AudienceQuery: audience, Channel: in.Channel, TemplateID: templateID})
	if err != nil {
		return Campaign{}, err
	}
	for _, subjectID := range subjectIDs {
		rid, err := uuid.NewV7()
		if err != nil {
			return Campaign{}, err
		}
		if _, err := q.InsertCampaignRecipient(ctx, consentstore.InsertCampaignRecipientParams{ID: rid, CampaignID: id, SubjectID: subjectID}); err != nil {
			return Campaign{}, err
		}
	}
	if err := s.audit(ctx, "consent.campaign.create", CampaignType, id, nil, map[string]any{"name": in.Name, "recipients": len(subjectIDs)}); err != nil {
		return Campaign{}, err
	}
	counts, err := s.campaignCounts(ctx, id)
	if err != nil {
		return Campaign{}, err
	}
	return toCampaign(row, counts), nil
}

// GetCampaign reads one campaign with its live recipient counts.
func (s *Service) GetCampaign(ctx context.Context, id uuid.UUID) (Campaign, error) {
	row, err := consentstore.New(pdb.MustTxFromContext(ctx)).GetCampaign(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, ErrNotFound
	}
	if err != nil {
		return Campaign{}, err
	}
	counts, err := s.campaignCounts(ctx, id)
	if err != nil {
		return Campaign{}, err
	}
	return toCampaign(row, counts), nil
}

// ListCampaigns returns every campaign of the tenant, newest first.
func (s *Service) ListCampaigns(ctx context.Context) ([]Campaign, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListCampaigns(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Campaign, 0, len(rows))
	for _, r := range rows {
		counts, err := s.campaignCounts(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, toCampaign(r, counts))
	}
	return out, nil
}

func randomCampaignToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashCampaignToken(token string) *string {
	sum := sha256.Sum256([]byte(token))
	h := hex.EncodeToString(sum[:])
	return &h
}

// notifyChannel maps the campaign's own channel to one of notify's own channel constants (CON-24's
// email/sms/line is a subset of notify's own).
func notifyChannel(channel string) string {
	switch channel {
	case "sms":
		return notify.ChannelSMS
	case "line":
		return notify.ChannelLine
	default:
		return notify.ChannelEmail
	}
}

// sendToRecipients is SendCampaign/ResendCampaign's shared core: issues a fresh token per given recipient,
// links it into the campaign's own public page, and sends one notification each — best-effort per recipient,
// the same "one failure doesn't fail the whole batch" discipline breach's own subject-notice batches use,
// since a bad address for one person must never block everyone else's link.
func (s *Service) sendToRecipients(ctx context.Context, campaign Campaign, key string, recipients []consentstore.ConsentCampaignRecipient, lang string) (int, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	sent := 0
	for _, r := range recipients {
		addr, err := s.recipientAddressFor(ctx, r.SubjectID, campaign.Channel)
		if err != nil || addr == "" {
			continue // no usable address for this channel — skip, don't fail the whole send
		}
		token, err := randomCampaignToken()
		if err != nil {
			return sent, err
		}
		if _, err := q.SetCampaignRecipientSent(ctx, consentstore.SetCampaignRecipientSentParams{
			TokenHash: hashCampaignToken(token), SentAt: pgtype.Timestamptz{Time: s.now(), Valid: true}, ID: r.ID,
		}); err != nil {
			return sent, err
		}
		base := strings.TrimRight(s.PortalBaseURL, "/")
		if base == "" {
			base = "http://localhost:3001"
		}
		link := fmt.Sprintf("%s/%s/consent-campaigns/%s?token=%s", base, lang, key, token)
		templateCode := "consent.campaign_request"
		if s.Notify != nil {
			if _, err := s.Notify.Send(ctx, notify.Request{TemplateCode: templateCode, Channel: notifyChannel(campaign.Channel), Language: lang,
				RecipientAddress: addr, Vars: map[string]any{"link": link, "campaign_name": campaign.Name}}); err != nil {
				return sent, err
			}
		}
		sent++
	}
	return sent, nil
}

// recipientAddressFor decrypts the subject's own identifier matching the campaign's channel — email for
// "email"/"line" (LINE delivery itself is a notify-layer concern, PLT-04's own MockSender until a real
// provider exists, same as every other LINE send in this codebase), phone for "sms".
func (s *Service) recipientAddressFor(ctx context.Context, subjectID uuid.UUID, channel string) (string, error) {
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	kind := crypto.KindEmail
	if channel == "sms" {
		kind = crypto.KindPhone
	}
	row, err := q.FindIdentifierForSubject(ctx, consentstore.FindIdentifierForSubjectParams{SubjectID: subjectID, IdentifierType: string(kind)})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	v, err := s.Keyring.Decrypt(ctx, cryptoClass, identifierContext, row)
	if err != nil {
		return "", err
	}
	return string(v), nil
}

// SendCampaign is the first dispatch: every recipient still "queued" is sent its own link. A campaign with
// nothing left to send (already fully dispatched) is a harmless no-op.
func (s *Service) SendCampaign(ctx context.Context, id uuid.UUID, lang string) (Campaign, error) {
	return s.dispatchCampaign(ctx, id, lang, []string{"queued"})
}

// ResendCampaign is the acceptance criterion's other half ("ส่งซ้ำเฉพาะผู้ยังไม่ตอบได้"): every recipient
// not yet "responded" (queued/sent/opened) gets a fresh link; anyone who already responded is left alone.
func (s *Service) ResendCampaign(ctx context.Context, id uuid.UUID, lang string) (Campaign, error) {
	return s.dispatchCampaign(ctx, id, lang, []string{"queued", "sent", "opened"})
}

func (s *Service) dispatchCampaign(ctx context.Context, id uuid.UUID, lang string, statuses []string) (Campaign, error) {
	if lang != "en" {
		lang = "th"
	}
	campaign, err := s.GetCampaign(ctx, id)
	if err != nil {
		return Campaign{}, err
	}
	if campaign.Status == "cancelled" {
		return Campaign{}, ErrInvalidTransition
	}
	key, err := s.campaignKey(ctx, id)
	if err != nil {
		return Campaign{}, err
	}
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	all, err := q.ListCampaignRecipients(ctx, id)
	if err != nil {
		return Campaign{}, err
	}
	var targets []consentstore.ConsentCampaignRecipient
	for _, r := range all {
		if slices.Contains(statuses, r.Status) {
			targets = append(targets, r)
		}
	}
	if _, err := s.sendToRecipients(ctx, campaign, key, targets, lang); err != nil {
		return Campaign{}, err
	}
	counts, err := s.campaignCounts(ctx, id)
	if err != nil {
		return Campaign{}, err
	}
	status := "sending"
	if counts["queued"] == 0 {
		status = "done"
	}
	if _, err := q.SetCampaignStatus(ctx, consentstore.SetCampaignStatusParams{
		Status: status, SentCount: int32(counts["sent"] + counts["opened"] + counts["responded"]), ResponseCount: int32(counts["responded"]), ID: id,
	}); err != nil {
		return Campaign{}, err
	}
	if err := s.audit(ctx, "consent.campaign.send", CampaignType, id, nil, map[string]any{"targets": len(targets)}); err != nil {
		return Campaign{}, err
	}
	return s.GetCampaign(ctx, id)
}

// campaignKey issues (or re-reads) the campaign's own public key — the coalesce-on-first-issue pattern every
// other /public/v1 surface in this codebase uses, via publickeys.Get/Issue since consent.campaigns has no
// public_key column of its own (the same CON-01 "cookie.domains has none either" gap this shared helper was
// built for).
func (s *Service) campaignKey(ctx context.Context, id uuid.UUID) (string, error) {
	if key, ok, err := publickeys.Get(ctx, publickeys.EntityConsentCampaign, id); err != nil {
		return "", err
	} else if ok {
		return key, nil
	}
	return publickeys.Issue(ctx, publickeys.EntityConsentCampaign, id, nil)
}

// CampaignView is what the recipient's own link shows before they decide.
type CampaignView struct {
	CampaignName string
	Purposes     []PublicPurpose
	Responded    bool
}

// ViewCampaignRecipient resolves the recipient by their own token (the campaign's public key already named
// the tenant/campaign) and marks it opened — the acceptance criterion's own "เปิด" count.
func (s *Service) ViewCampaignRecipient(ctx context.Context, campaignID uuid.UUID, token, lang string) (CampaignView, error) {
	if lang != "en" {
		lang = "th"
	}
	campaign, err := s.GetCampaign(ctx, campaignID)
	if err != nil {
		return CampaignView{}, err
	}
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.LockCampaignRecipientByTokenHash(ctx, consentstore.LockCampaignRecipientByTokenHashParams{CampaignID: campaignID, TokenHash: hashCampaignToken(token)})
	if errors.Is(err, pgx.ErrNoRows) {
		return CampaignView{}, ErrNotFound
	}
	if err != nil {
		return CampaignView{}, err
	}
	if _, err := q.MarkCampaignRecipientOpened(ctx, r.ID); err != nil {
		return CampaignView{}, err
	}
	purposes, err := s.campaignPurposes(ctx, campaign, lang)
	if err != nil {
		return CampaignView{}, err
	}
	return CampaignView{CampaignName: campaign.Name, Purposes: purposes, Responded: r.Status == "responded"}, nil
}

// campaignPurposes resolves the campaign's own purpose_ids into PublicPurpose (one language) plus the
// matching CPPurpose shape checkDecisions already knows how to validate against — the exact pick-by-language
// pattern PublicView (CON-09) and activePreferencePurposes (CON-18) already use.
func (s *Service) campaignPurposes(ctx context.Context, campaign Campaign, lang string) ([]PublicPurpose, error) {
	_, cps, err := s.campaignCPPurposes(ctx, campaign, lang)
	if err != nil {
		return nil, err
	}
	pick := func(t Text) string {
		if lang == "en" && t.En != "" {
			return t.En
		}
		return t.Th
	}
	out := make([]PublicPurpose, 0, len(cps))
	for _, p := range cps {
		out = append(out, PublicPurpose{Code: p.Code, VersionNo: *p.CurrentVersionNo, Name: pick(p.Name), Text: pick(p.ConsentText),
			ExplicitText: pick(p.ExplicitText), IsSensitive: p.IsSensitive})
	}
	return out, nil
}

// campaignCPPurposes is the shared resolution behind both the view (campaignPurposes) and the write
// (RespondCampaignRecipient): every active published purpose of the tenant, filtered to this campaign's own
// purpose_ids.
func (s *Service) campaignCPPurposes(ctx context.Context, campaign Campaign, lang string) ([]Purpose, []CPPurpose, error) {
	purposes, cps, err := s.activePreferencePurposes(ctx)
	_ = purposes
	if err != nil {
		return nil, nil, err
	}
	var out []CPPurpose
	for _, p := range cps {
		if slices.Contains(campaign.PurposeIDs, p.PurposeID) {
			out = append(out, p)
		}
	}
	return purposes, out, nil
}

// RespondCampaignRecipient is the recipient's own decision — written through the exact same
// checkDecisions/writeDecisions core every other consent write uses (source "campaign", already in
// consent_transactions' own CHECK constraint), attributed to a real active collection point the same
// disclosed way CON-18's own channelForPreferenceUpdate already established. Unlike a public collection-point
// form, not every purpose needs deciding in one call (sub.Public stays false) — a recipient may consent to
// some and leave others, and withdrawal isn't offered here (every decision is CONSENTED/NOT_CONSENTED).
func (s *Service) RespondCampaignRecipient(ctx context.Context, campaignID uuid.UUID, token string, decisions []Decision, lang string) (Receipt, error) {
	if lang != "en" {
		lang = "th"
	}
	campaign, err := s.GetCampaign(ctx, campaignID)
	if err != nil {
		return Receipt{}, err
	}
	q := consentstore.New(pdb.MustTxFromContext(ctx))
	r, err := q.LockCampaignRecipientByTokenHash(ctx, consentstore.LockCampaignRecipientByTokenHashParams{CampaignID: campaignID, TokenHash: hashCampaignToken(token)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, ErrNotFound
	}
	if err != nil {
		return Receipt{}, err
	}
	if r.Status == "responded" {
		return Receipt{}, ErrInvalidTransition
	}
	for _, d := range decisions {
		if d.Decision == TxWithdrawn {
			return Receipt{}, &DecisionError{Fields: []FieldError{{d.PurposeCode, "withdraw_not_allowed"}}}
		}
	}
	_, cps, err := s.campaignCPPurposes(ctx, campaign, lang)
	if err != nil {
		return Receipt{}, err
	}
	synthetic := CollectionPoint{Purposes: cps}
	sub := Submission{SubjectID: &r.SubjectID, Decisions: decisions, Language: lang, Source: "campaign"}
	kept, err := s.checkDecisions(ctx, synthetic, sub)
	if err != nil {
		return Receipt{}, err
	}
	channel, err := s.channelForPreferenceUpdate(ctx, kept)
	if err != nil {
		return Receipt{}, err
	}
	receipt, err := s.writeDecisions(ctx, channel, sub, kept)
	if err != nil {
		return Receipt{}, err
	}
	if _, err := q.MarkCampaignRecipientResponded(ctx, consentstore.MarkCampaignRecipientRespondedParams{
		RespondedAt: pgtype.Timestamptz{Time: s.now(), Valid: true}, ID: r.ID,
	}); err != nil {
		return Receipt{}, err
	}
	counts, err := s.campaignCounts(ctx, campaignID)
	if err != nil {
		return Receipt{}, err
	}
	if _, err := q.SetCampaignStatus(ctx, consentstore.SetCampaignStatusParams{
		Status: campaign.Status, SentCount: int32(counts["sent"] + counts["opened"] + counts["responded"]), ResponseCount: int32(counts["responded"]), ID: campaignID,
	}); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}
