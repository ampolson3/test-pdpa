package docs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/docs/render"
	docsstore "pdpa-platform/internal/platform/docs/store"
	"pdpa-platform/internal/platform/jobs"
)

const renderAttempts = 5

// RenderArgs is docs.render: produce the PDF and DOCX of a published version in each of its languages.
type RenderArgs struct {
	jobs.TenantArgs
	VersionID string `json:"version_id"`
}

func (RenderArgs) Kind() string { return "docs.render" }

// Renderer works docs.render. The files are attached to the version (VersionEntityType), so downloading them needs
// the document type's read permission.
type Renderer struct {
	river.WorkerDefaults[RenderArgs]
	Service *Service
	Logger  *slog.Logger
}

func (w *Renderer) Timeout(*river.Job[RenderArgs]) time.Duration { return 5 * time.Minute }

func (w *Renderer) Work(ctx context.Context, job *river.Job[RenderArgs]) error {
	id, err := uuid.Parse(job.Args.VersionID)
	if err != nil {
		return river.JobCancel(err)
	}
	q := docsstore.New(pdb.MustTxFromContext(ctx))
	v, err := q.GetDocumentVersion(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}
	if v.RenderStatus == "done" {
		return nil
	}
	doc, err := q.GetDocument(ctx, v.DocumentID)
	if err != nil {
		return err
	}
	err = w.render(ctx, q, doc.DocType, v)
	if err != nil && job.Attempt >= job.MaxAttempts {
		// Out of attempts: say so on the version (the UI shows it) instead of leaving it pending forever.
		w.logger().Error("docs: render failed", "alert", "document_render_failed", "version_id", id, "error", err)
		return q.SetVersionRenderStatus(ctx, docsstore.SetVersionRenderStatusParams{ID: id, RenderStatus: "failed"})
	}
	return err
}

func (w *Renderer) render(ctx context.Context, q *docsstore.Queries, docType string, v docsstore.GetDocumentVersionRow) error {
	var f frozen
	if err := json.Unmarshal(v.Content, &f); err != nil {
		return river.JobCancel(err)
	}
	if w.Service.PDF == nil {
		return render.ErrNoRenderer
	}
	ids := map[string]pgtype.UUID{}
	for lang, in := range buildInputs(f) {
		// A savepoint per language keeps the job's transaction usable to record a final failure.
		err := pdb.Savepoint(ctx, func(ctx context.Context) error {
			pdf, err := w.Service.PDF.PDF(ctx, render.HTML(in))
			if err != nil {
				return err
			}
			docx, err := render.DOCX(in)
			if err != nil {
				return err
			}
			base := fmt.Sprintf("%s-v%d-%s", f.Title, v.VersionNo, lang)
			for format, body := range map[string][]byte{"pdf": pdf, "docx": docx} {
				file, err := w.Service.Files.SaveGenerated(ctx, base+"."+format, bytes.NewReader(body), VersionEntityType(docType), v.ID)
				if err != nil {
					return err
				}
				ids[lang+"."+format] = pgtype.UUID{Bytes: file.ID, Valid: true}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return q.SetVersionFiles(ctx, docsstore.SetVersionFilesParams{ID: v.ID, RenderStatus: "done",
		PdfFileID: ids["th.pdf"], DocxFileID: ids["th.docx"], PdfEnFileID: ids["en.pdf"], DocxEnFileID: ids["en.docx"]})
}

func (w *Renderer) logger() *slog.Logger {
	if w.Logger != nil {
		return w.Logger
	}
	return slog.Default()
}
