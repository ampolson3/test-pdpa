package service_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	dataflowservice "pdpa-platform/internal/dataflow/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/platform/docs/render"
	ropaservice "pdpa-platform/internal/ropa/service"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

// seedExportFixture builds one org unit with one activity that has a data category, a processor recipient
// tracked as a vendor with no DPA (DFG-04's own warning) and a foreign recipient with no logged transfer
// (DFG-03's own warning), plus a disposal method — enough of every node/edge type DFG-08's export and report
// need to cover in one fixture.
func seedExportFixture(t *testing.T, e env) ropaservice.Activity {
	t.Helper()
	var a ropaservice.Activity
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "ฝ่ายไอที", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err = e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "IT-01", Name: "ระบบคลาวด์", Role: "controller"}, 0)
		if err != nil {
			return err
		}
		vendorParty, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการคลาวด์", CountryCode: "TH"}, 0)
		if err != nil {
			return err
		}
		if _, err := e.vendor.SaveVendor(ctx, vendorservice.Vendor{PartyID: vendorParty.ID, ServiceDescription: "ประมวลผลข้อมูลบนคลาวด์", IsProcessor: true}, 0); err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: a.ID, PartyID: vendorParty.ID, RecipientRole: "processor"}); err != nil {
			return err
		}
		foreignParty, err := e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้รับข้อมูลต่างประเทศ", CountryCode: "SG"}, 0)
		if err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: a.ID, PartyID: foreignParty.ID, RecipientRole: "processor"}); err != nil {
			return err
		}
		_, err = e.ropa.AddRetentionRule(ctx, ropaservice.RetentionRule{ActivityID: a.ID, RetentionBasis: "ตามกฎหมาย", TriggerEvent: "สิ้นสุดสัญญา", DisposalMethod: "destroy"})
		return err
	})
	return a
}

// TestExportDiagram_SVGContainsWarnings is DFG-08's own acceptance criterion read as "the exported image
// matches the diagram": the SVG carries the activity, the processor recipient's missing-DPA warning colour
// and the foreign recipient's missing-transfer-basis warning colour — the same red (#dc2626) the on-screen
// React Flow page already uses (data-flow-content.tsx).
func TestExportDiagram_SVGContainsWarnings(t *testing.T) {
	e := setup(t, "dfgexportsvg")
	seedExportFixture(t, e)

	e.in(t, func(ctx context.Context) error {
		b, name, err := e.svc.ExportDiagram(ctx, nil, dataflowservice.FormatSVG, dataflowservice.PaperA3)
		if err != nil {
			t.Fatal(err)
		}
		if name != "dataflow.svg" {
			t.Errorf("name = %q", name)
		}
		s := string(b)
		if !strings.HasPrefix(s, "<svg") {
			t.Errorf("not an svg document: %q", s[:min(40, len(s))])
		}
		if !strings.Contains(s, "ระบบคลาวด์") {
			t.Error("missing activity label")
		}
		if !strings.Contains(s, "ผู้ให้บริการคลาวด์") {
			t.Error("missing processor recipient label")
		}
		if strings.Count(s, "#dc2626") < 2 {
			t.Errorf("expected at least two warning-coloured elements (missing DPA + missing transfer basis), got: %s", s)
		}
		return nil
	})
}

func TestExportDiagram_InvalidFormat(t *testing.T) {
	e := setup(t, "dfgexportfmt")
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.ExportDiagram(ctx, nil, "bmp", dataflowservice.PaperA3); !errors.Is(err, dataflowservice.ErrInvalid) {
			t.Errorf("err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestExportDiagram_InvalidPaperForPDF(t *testing.T) {
	e := setup(t, "dfgexportpaper")
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.ExportDiagram(ctx, nil, dataflowservice.FormatPDF, "poster"); !errors.Is(err, dataflowservice.ErrInvalid) {
			t.Errorf("err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestExportDiagram_NoRendererConfigured proves the test harness's own nil PDF/Image renderers fail each
// raster/paginated format with the right sentinel while SVG — which never needs one — still works.
func TestExportDiagram_NoRendererConfigured(t *testing.T) {
	e := setup(t, "dfgexportnorender")
	seedExportFixture(t, e)

	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.ExportDiagram(ctx, nil, dataflowservice.FormatPNG, dataflowservice.PaperA3); !errors.Is(err, render.ErrNoImageRenderer) {
			t.Errorf("png err = %v, want ErrNoImageRenderer", err)
		}
		if _, _, err := e.svc.ExportDiagram(ctx, nil, dataflowservice.FormatPDF, dataflowservice.PaperA3); !errors.Is(err, render.ErrNoRenderer) {
			t.Errorf("pdf err = %v, want ErrNoRenderer", err)
		}
		if _, _, err := e.svc.ExportDiagram(ctx, nil, dataflowservice.FormatSVG, dataflowservice.PaperA3); err != nil {
			t.Errorf("svg err = %v, want nil", err)
		}
		return nil
	})
}

// TestExportDiagram_ChromiumRendersPNGAndPDF is a live smoke test against the pre-installed headless
// Chromium (the environment's own CHROMIUM_PATH convention, as render_test.go's own
// TestChromiumPDF_EmbedsThaiFont already establishes) — skipped when it isn't configured.
func TestExportDiagram_ChromiumRendersPNGAndPDF(t *testing.T) {
	path := os.Getenv("CHROMIUM_PATH")
	if path == "" {
		t.Skip("CHROMIUM_PATH not set")
	}
	e := setup(t, "dfgexportchromium")
	seedExportFixture(t, e)
	e.svc.PDF = &render.Chromium{Path: path}
	e.svc.Image = &render.Chromium{Path: path}

	e.in(t, func(ctx context.Context) error {
		png, name, err := e.svc.ExportDiagram(ctx, nil, dataflowservice.FormatPNG, dataflowservice.PaperA3)
		if err != nil {
			t.Fatal(err)
		}
		if name != "dataflow.png" || len(png) < 100 || string(png[1:4]) != "PNG" {
			t.Errorf("not a PNG: name=%q len=%d", name, len(png))
		}
		pdf, name, err := e.svc.ExportDiagram(ctx, nil, dataflowservice.FormatPDF, dataflowservice.PaperA3)
		if err != nil {
			t.Fatal(err)
		}
		if name != "dataflow.pdf" || !strings.HasPrefix(string(pdf), "%PDF") {
			t.Errorf("not a PDF: name=%q len=%d", name, len(pdf))
		}
		return nil
	})
}

// TestReport_CoversActivityRecipientsTransfersAndDisposal is DFG-08's own "รายงานประกอบ": one text report
// built from the exact same live graph as the image, so it is guaranteed to agree with it.
func TestReport_CoversActivityRecipientsTransfersAndDisposal(t *testing.T) {
	e := setup(t, "dfgreport")
	seedExportFixture(t, e)

	e.in(t, func(ctx context.Context) error {
		b, name, err := e.svc.Report(ctx, nil, "th", "html")
		if err != nil {
			t.Fatal(err)
		}
		if name != "dataflow-report-th.html" {
			t.Errorf("name = %q", name)
		}
		s := string(b)
		for _, want := range []string{"ระบบคลาวด์", "ผู้ให้บริการคลาวด์", "ไม่มี DPA", "ผู้รับข้อมูลต่างประเทศ", "ไม่มีฐานการโอน", "destroy"} {
			if !strings.Contains(s, want) {
				t.Errorf("report missing %q", want)
			}
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		b, _, err := e.svc.Report(ctx, nil, "en", "html")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "no DPA on file") || !strings.Contains(string(b), "no transfer basis logged") {
			t.Error("english report missing warning text")
		}
		return nil
	})
}

func TestReport_NoActivitiesIsEmptyNotError(t *testing.T) {
	e := setup(t, "dfgreportempty")
	e.in(t, func(ctx context.Context) error {
		b, _, err := e.svc.Report(ctx, nil, "th", "html")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "ไม่พบกิจกรรมที่ตรงกับเงื่อนไข") {
			t.Error("expected the no-activities message")
		}
		return nil
	})
}

func TestReport_InvalidLanguageAndFormat(t *testing.T) {
	e := setup(t, "dfgreportinvalid")
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.Report(ctx, nil, "fr", "html"); !errors.Is(err, dataflowservice.ErrInvalid) {
			t.Errorf("language err = %v, want ErrInvalid", err)
		}
		if _, _, err := e.svc.Report(ctx, nil, "th", "xml"); !errors.Is(err, dataflowservice.ErrInvalid) {
			t.Errorf("format err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestReport_PDFWithoutRenderer proves the report's own pdf path fails the same way ExportDiagram's does when
// no renderer is configured, while docx/html never need one.
func TestReport_PDFWithoutRenderer(t *testing.T) {
	e := setup(t, "dfgreportnorender")
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.Report(ctx, nil, "th", "pdf"); !errors.Is(err, render.ErrNoRenderer) {
			t.Errorf("err = %v, want ErrNoRenderer", err)
		}
		if _, _, err := e.svc.Report(ctx, nil, "th", "docx"); err != nil {
			t.Errorf("docx err = %v, want nil", err)
		}
		return nil
	})
}
