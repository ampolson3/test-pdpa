package wiring

import (
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/platform/notify"
)

// Forms builds the form & assessment engine (PLT-06) with the form types whose permissions exist: each
// type uses the codes of the module that owns it (no permission of its own). Types without an owning
// module's codes (breach, intake, quiz) are not offered until that module registers them.
func Forms(n *notify.Service, a *audit.Service) *forms.Service {
	s := &forms.Service{Notify: n, Audit: a}
	s.Register("assessment", forms.Policy{Read: "assessment.template.read", Create: "assessment.template.create",
		Update: "assessment.template.update", Publish: "assessment.template.publish", Respond: "assessment.dpia.create"})
	s.Register("questionnaire", forms.Policy{Read: "ropa.template.read", Create: "ropa.template.create",
		Update: "ropa.template.update", Publish: "ropa.template.publish", Respond: "ropa.activity.update"})
	s.Register("dsar", forms.Policy{Read: "dsar.form.read", Create: "dsar.form.create",
		Update: "dsar.form.update", Publish: "dsar.form.publish", Respond: "dsar.request.create"})
	// Consent forms are answered by data subjects through the public API (forms.Service.Record), never here.
	s.Register("consent", forms.Policy{Read: "consent.collectionpoint.read", Create: "consent.collectionpoint.create",
		Update: "consent.collectionpoint.update", Publish: "consent.collectionpoint.publish"})
	// Breach risk assessments (BRE-05): the DPO designs and publishes them; the incident team answers them through the
	// breach module (forms.Service.Record), with bands none / low / high.
	s.Register("breach", forms.Policy{Read: "breach.incident.read", Create: "breach.incident.approve",
		Update: "breach.incident.approve", Publish: "breach.incident.approve", Respond: "breach.incident.update"})
	return s
}
