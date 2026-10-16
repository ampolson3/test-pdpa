package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/crypto"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// typeByCode finds a seeded DSAR-13 request type (migration 00043) by its fixed code.
func typeByCode(t *testing.T, ctx context.Context, e env, code string) uuid.UUID {
	t.Helper()
	types, err := e.svc.ListRequestTypes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, rt := range types {
		if rt.Code == code {
			return rt.ID
		}
	}
	t.Fatalf("no seeded request type with code %q", code)
	return uuid.Nil
}

func seedAsset(t *testing.T, ctx context.Context, e env, name string) uuid.UUID {
	t.Helper()
	a, err := e.ropa.SaveAsset(ctx, ropaservice.Asset{Name: name, AssetType: "application", Status: "active"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

// createAndAdvanceToInReview is DSAR-03's own test fixture: every right-type/subtask test needs a request
// already past verifying/in_review before in_progress is reachable (ST-02).
func createAndAdvanceToInReview(t *testing.T, ctx context.Context, e env, typeID, leID uuid.UUID, dataSource *string) dsarservice.Request {
	t.Helper()
	r, err := e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
		Channel: "email", RequesterName: "ทดสอบ สิทธิ", RequesterContact: "rights-test@example.com", ContactKind: crypto.KindEmail, DataSource: dataSource})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "verifying"}); err != nil {
		t.Fatal(err)
	}
	r2, _, err := e.svc.Transition(ctx, r.ID, r.RowVersion+1, dsarservice.TransitionInput{To: "in_review"})
	if err != nil {
		t.Fatal(err)
	}
	return r2
}

// TestTransition_InProgress_OpensOnePerRightSubtaskPerAsset is DSAR-03's own acceptance criterion: every
// right type gets its own per-system subtask when entering in_progress with linked RoPA assets — an access
// request opens "search" subtasks, an erasure request opens "delete" subtasks, one per asset.
func TestTransition_InProgress_OpensOnePerRightSubtaskPerAsset(t *testing.T) {
	e := setup(t, "dsaraccesssubtask")
	var leID, accessTypeID, erasureTypeID uuid.UUID
	var asset1, asset2 uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, _ = seedLegalEntityAndType(t, ctx, e)
		accessTypeID = typeByCode(t, ctx, e, "access")
		erasureTypeID = typeByCode(t, ctx, e, "erasure")
		asset1 = seedAsset(t, ctx, e, "ระบบ CRM")
		asset2 = seedAsset(t, ctx, e, "ระบบ HR")
		return nil
	})

	var accessReq dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		accessReq = createAndAdvanceToInReview(t, ctx, e, accessTypeID, leID, nil)
		if _, _, err := e.svc.Transition(ctx, accessReq.ID, accessReq.RowVersion, dsarservice.TransitionInput{To: "in_progress", AssetIDs: []uuid.UUID{asset1, asset2}}); err != nil {
			return err
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		subs, err := e.svc.ListSubtasks(ctx, accessReq.ID)
		if err != nil {
			return err
		}
		if len(subs) != 2 {
			t.Fatalf("subtasks = %d, want 2", len(subs))
		}
		seenAssets := map[uuid.UUID]bool{}
		for _, st := range subs {
			if st.Action != "search" {
				t.Errorf("access subtask action = %q, want search", st.Action)
			}
			if st.AssetID == nil {
				t.Fatal("expected asset_id on the auto-opened subtask")
			}
			seenAssets[*st.AssetID] = true
		}
		if !seenAssets[asset1] || !seenAssets[asset2] {
			t.Error("expected one subtask per linked asset")
		}
		return nil
	})

	var erasureReq dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		erasureReq = createAndAdvanceToInReview(t, ctx, e, erasureTypeID, leID, nil)
		_, _, err := e.svc.Transition(ctx, erasureReq.ID, erasureReq.RowVersion, dsarservice.TransitionInput{To: "in_progress", AssetIDs: []uuid.UUID{asset1}})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		subs, err := e.svc.ListSubtasks(ctx, erasureReq.ID)
		if err != nil {
			return err
		}
		if len(subs) != 1 || subs[0].Action != "delete" {
			t.Fatalf("erasure subtasks = %v, want exactly one delete subtask", subs)
		}
		return nil
	})
}

// TestTransition_InProgress_UnknownAssetRefused proves the FK-visibility check (rule 1): an asset id from
// another tenant (or that doesn't exist) is refused rather than silently opening a subtask against it.
func TestTransition_InProgress_UnknownAssetRefused(t *testing.T) {
	e := setup(t, "dsarbadasset")
	var leID, typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		return nil
	})
	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		r = createAndAdvanceToInReview(t, ctx, e, typeID, leID, nil)
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		_, _, err := e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "in_progress", AssetIDs: []uuid.UUID{uuid.New()}})
		if !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestTransition_Completed_PublishesDsarCompletedEvent is DSAR-03's ม.32/34 "แจ้งระบบปลายทาง" half: closing
// any request publishes the already-cataloged dsar.completed event for downstream systems to subscribe to.
func TestTransition_Completed_PublishesDsarCompletedEvent(t *testing.T) {
	e := setup(t, "dsarcompletedevent")
	var leID, typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		return nil
	})
	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		r = createAndAdvanceToInReview(t, ctx, e, typeID, leID, nil)
		r2, _, err := e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "in_progress"})
		if err != nil {
			return err
		}
		r = r2
		outcome := "fulfilled"
		_, _, err = e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "completed", Outcome: &outcome})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		var n int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*)::int FROM platform.outbox_events WHERE event_type = 'dsar.completed' AND aggregate_id = $1`, r.ID).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("dsar.completed outbox rows = %d, want 1", n)
		}
		return nil
	})
}

// TestGenerateResponseLetter_AccessDisclosesDataSource is the ม.30 half: when an access request names a
// data source not collected directly from the subject, the generated result letter discloses it verbatim.
func TestGenerateResponseLetter_AccessDisclosesDataSource(t *testing.T) {
	e := setup(t, "dsardatasource")
	var leID, typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, _ = seedLegalEntityAndType(t, ctx, e)
		typeID = typeByCode(t, ctx, e, "access")
		return nil
	})
	source := "ได้รับจากบริษัทพันธมิตร ABC จำกัด"
	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		r = createAndAdvanceToInReview(t, ctx, e, typeID, leID, &source)
		r2, _, err := e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "in_progress"})
		if err != nil {
			return err
		}
		r = r2
		return nil
	})
	var docID *uuid.UUID
	e.in(t, func(ctx context.Context) error {
		outcome := "fulfilled"
		_, id, err := e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "completed", Outcome: &outcome})
		docID = id
		return err
	})
	if docID == nil {
		t.Fatal("expected a result letter document")
	}
	e.in(t, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, *docID)
		if err != nil {
			return err
		}
		var sb strings.Builder
		plainText(doc.Draft.Content["th"], &sb)
		if !strings.Contains(sb.String(), source) {
			t.Error("expected the disclosed data source in the access response letter")
		}
		return nil
	})
}
