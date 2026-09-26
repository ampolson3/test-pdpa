package wiring

import (
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	orgservice "pdpa-platform/internal/org/service"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/files"
	"pdpa-platform/internal/platform/versioning"
)

// Docs builds the document composer (PLT-16) with the document types whose modules have permission codes. Each
// type borrows its module's codes (no permissions of its own), and every version is checked by a DPO before it is
// published. report / other have no owning module yet and are not offered. Call RegisterVersioning on the result
// once the versioning service is built (the API does; the worker only renders).
func Docs(v *versioning.Service, f *files.Service, r *river.Client[pgx.Tx], a *audit.Service, pdf render.PDFRenderer) *docs.Service {
	s := &docs.Service{Versioning: v, Files: f, River: r, Audit: a, Org: &orgservice.Service{Audit: a}, PDF: pdf}
	notice := docs.Policy{Read: "notice.document.read", Create: "notice.document.create", Update: "notice.document.update",
		Publish: "notice.document.publish", Approver: "DPO", TemplateRead: "notice.template.read", TemplateWrite: "notice.template.update"}
	s.Register("notice", notice)
	s.Register("policy", notice)
	s.Register("dpa", docs.Policy{Read: "agreement.dpa.read", Create: "agreement.dpa.create", Update: "agreement.dpa.update",
		Publish: "agreement.dpa.publish", Approver: "DPO", TemplateRead: "agreement.dpa.read", TemplateWrite: "agreement.dpa.update"})
	s.Register("dsa", docs.Policy{Read: "agreement.dsa.read", Create: "agreement.dsa.create", Update: "agreement.dsa.update",
		Publish: "agreement.dsa.publish", Approver: "DPO", TemplateRead: "agreement.dsa.read", TemplateWrite: "agreement.dsa.update"})
	// A reply letter is part of handling the request; releasing it is the DPO's approval of the reply.
	s.Register("dsar_letter", docs.Policy{Read: "dsar.request.read", Create: "dsar.request.update", Update: "dsar.request.update",
		Publish: "dsar.request.approve", Approver: "DPO", TemplateRead: "dsar.request.read", TemplateWrite: "dsar.request.approve"})
	breach := docs.Policy{Read: "breach.notification.read", Create: "breach.notification.create", Update: "breach.notification.update",
		Publish: "breach.notification.publish", Approver: "DPO", TemplateRead: "breach.notification.read", TemplateWrite: "breach.notification.approve"}
	s.Register("pdpc_form", breach)
	s.Register("breach_letter", breach)
	return s
}
