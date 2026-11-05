package service_test

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/jobs"
)

func annexEnvOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// fakeDataflow stands in for the real dataflow module — DSA-07's own acceptance criterion is about the
// annex attaching whatever the data-flow renderer currently returns, not about re-testing DFG-08's own
// Chromium/Gotenberg rendering path, which already has its own tests.
type fakeDataflow struct{ png []byte }

func (f fakeDataflow) ExportDiagram(ctx context.Context, legalEntityID *uuid.UUID, format, paper string) ([]byte, string, error) {
	return f.png, "dataflow.png", nil
}

// needsFilesStack wires a real files.Service (S3 + a real River insert client) into e.svc, skipping the
// test when S3 isn't reachable here — the same reachability check docstest/breachtest's own fixtures use.
func needsFilesStack(t *testing.T, e env) {
	t.Helper()
	endpoint := annexEnvOr("TEST_S3_ENDPOINT", "127.0.0.1:8333")
	if c, err := net.DialTimeout("tcp", endpoint, time.Second); err != nil {
		t.Skipf("needs S3 (%s unreachable)", endpoint)
	} else {
		c.Close()
	}
	store, err := files.NewS3Store(files.S3Config{Endpoint: endpoint, AccessKey: annexEnvOr("TEST_S3_ACCESS_KEY", "pdpa-dev"),
		SecretKey: annexEnvOr("TEST_S3_SECRET_KEY", "pdpa-dev-secret-key"), Bucket: "pdpa-files-test", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	client, err := jobs.NewInsertClient(e.app)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Files = &files.Service{Store: store, River: client, Config: files.DefaultConfig()}
}

// TestAttachFlowDiagramAnnex_ReplacesThePreviousOne is the acceptance criterion directly
// ("ภาคผนวกแนบภาพแผนผังล่าสุดได้"): attaching renders and attaches a flow_diagram annex, and a second
// attach replaces it with the newer file rather than accumulating a history.
func TestAttachFlowDiagramAnnex_ReplacesThePreviousOne(t *testing.T) {
	e := setup(t, "dsa07annex")
	needsFilesStack(t, e)

	var agreementID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, _, activityID := fixture(t, ctx, e)
		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ประมวลผล", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		a, err := e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dpa", OurRole: "controller", CounterpartyPartyID: party.ID,
			LegalEntityID: leID, ActivityIDs: []uuid.UUID{activityID}, Title: "DPA ทดสอบภาคผนวก",
		})
		agreementID = a.ID
		return err
	})

	e.svc.Dataflow = fakeDataflow{png: []byte("first-png-bytes")}
	var first agreementservice.Annex
	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.AttachFlowDiagramAnnex(ctx, agreementID)
		first = a
		return err
	})
	if first.AnnexType != agreementservice.AnnexTypeFlowDiagram {
		t.Fatalf("annex_type = %q, want flow_diagram", first.AnnexType)
	}
	if first.FileID == nil {
		t.Fatal("expected a file_id on the attached annex")
	}

	e.svc.Dataflow = fakeDataflow{png: []byte("second-png-bytes-newer")}
	var second agreementservice.Annex
	e.in(t, func(ctx context.Context) error {
		a, err := e.svc.AttachFlowDiagramAnnex(ctx, agreementID)
		second = a
		return err
	})
	if second.FileID == nil || *second.FileID == *first.FileID {
		t.Fatal("expected the second attach to point at a newer file")
	}

	e.in(t, func(ctx context.Context) error {
		as, err := e.svc.ListAnnexes(ctx, agreementID)
		if err != nil {
			return err
		}
		var flowDiagrams int
		for _, a := range as {
			if a.AnnexType == agreementservice.AnnexTypeFlowDiagram {
				flowDiagrams++
				if a.FileID == nil || *a.FileID != *second.FileID {
					t.Errorf("remaining flow_diagram annex points at %v, want the second file %v", a.FileID, second.FileID)
				}
			}
		}
		if flowDiagrams != 1 {
			t.Errorf("found %d flow_diagram annexes, want exactly 1 (the latest)", flowDiagrams)
		}
		return nil
	})
}

func TestAttachFlowDiagramAnnex_NoRendererConfigured(t *testing.T) {
	e := setup(t, "dsa07norender")
	var agreementID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, _, activityID := fixture(t, ctx, e)
		party, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ประมวลผล", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		a, err := e.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dpa", OurRole: "controller", CounterpartyPartyID: party.ID,
			LegalEntityID: leID, ActivityIDs: []uuid.UUID{activityID}, Title: "DPA ไม่มี renderer",
		})
		agreementID = a.ID
		return err
	})
	// e.svc.Dataflow is left nil — the shared fixture never sets it.
	err := e.inAs(e.tenant.UserID, permissions, nil, func(ctx context.Context) error {
		_, err := e.svc.AttachFlowDiagramAnnex(ctx, agreementID)
		return err
	})
	if err == nil {
		t.Fatal("expected an error with no Dataflow configured")
	}
}

func TestListAnnexes_EmptyAndTwoTenantIsolation(t *testing.T) {
	a := setup(t, "dsa07isoa")
	b := setup(t, "dsa07isob")
	var agreementID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		leID, _, activityID := fixture(t, ctx, a)
		party, err := a.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ประมวลผล", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		ag, err := a.svc.CreateWizard(ctx, agreementservice.CreateInput{
			AgreementType: "dpa", OurRole: "controller", CounterpartyPartyID: party.ID,
			LegalEntityID: leID, ActivityIDs: []uuid.UUID{activityID}, Title: "DPA tenant A",
		})
		agreementID = ag.ID
		if err != nil {
			return err
		}
		as, err := a.svc.ListAnnexes(ctx, agreementID)
		if err != nil {
			return err
		}
		if len(as) != 0 {
			t.Errorf("fresh agreement has %d annexes, want 0", len(as))
		}
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		_, err := b.svc.ListAnnexes(ctx, agreementID)
		if err == nil {
			t.Error("tenant B saw tenant A's agreement")
		}
		return nil
	})
}
