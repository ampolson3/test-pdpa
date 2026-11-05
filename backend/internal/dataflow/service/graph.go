package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"

	agreementservice "pdpa-platform/internal/agreement/service"
	ropaservice "pdpa-platform/internal/ropa/service"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

// Node is one box on the diagram.
type Node struct {
	ID              string
	Type            string // org_unit | legal_entity | data_category | activity | recipient | country | disposal
	Label           string
	ParentID        string
	ActivityCode    string
	ActivityStatus  string
	Role            string
	Purposes        []string
	DataCategories  []string
	RecipientRole   string
	DisclosureBasis string
	CountryCode     string
	DisposalMethods []string
	// Warning is DFG-04's own flag (processor recipient tracked as a VEN-01 vendor with no "dpa" agreement —
	// a processor never tracked as a vendor at all isn't flagged, nothing to click through to) or DSA-12's
	// twin for a "controller"/"joint_controller" recipient with no "dsa" agreement naming it.
	Warning      bool
	VendorID     *uuid.UUID
	AgreementIDs []uuid.UUID
}

// Edge is one arrow on the diagram.
type Edge struct {
	ID      string
	Source  string
	Target  string
	Type    string // send | transfer | destroy
	Label   string
	Warning bool
}

// Graph is DFG-01's whole result.
type Graph struct {
	Nodes         []Node
	Edges         []Edge
	ActivityCount int
	GeneratedAt   time.Time
}

func orgUnitNodeID(id uuid.UUID) string      { return "org_unit:" + id.String() }
func legalEntityNodeID(id uuid.UUID) string  { return "legal_entity:" + id.String() }
func dataCategoryNodeID(id uuid.UUID) string { return "data_category:" + id.String() }
func activityNodeID(id uuid.UUID) string     { return "activity:" + id.String() }
func recipientNodeID(id uuid.UUID) string    { return "recipient:" + id.String() }
func countryNodeID(code string) string       { return "country:" + code }
func disposalNodeID(activityID uuid.UUID, method string) string {
	return "disposal:" + activityID.String() + ":" + method
}

// uncategorizedDataCategoryNodeID is DFG-06's own data_category view's swimlane for an activity whose
// ropa.activity_data names no category at all — never a silently empty parent_id.
const uncategorizedDataCategoryNodeID = "data_category:uncategorized"

func isForeignCountry(code string) bool { return code != "" && code != "TH" }

// View is DFG-06's own set of swimlane groupings for the same underlying diagram ("มุมมองรายแผนก รายระบบ
// รายบริษัท และรายประเภทข้อมูล" — department/system/company/data-category; "system" is deliberately not
// built here, the same "no consumer yet" deferral this codebase uses throughout: ROPA-02/03 never built any
// activity-to-ropa.assets link for a view to group by, per DPIA-04's own disclosed gap). Every view computes
// from the exact same activities/recipients/countries/disposal methods — only which swimlane an activity's
// own ParentID points at changes, so "สลับมุมมองได้โดยข้อมูลตรงกัน" (switching views keeps the data
// consistent) holds by construction: nodes/edges beyond the swimlane itself are identical across views.
type View string

const (
	// ViewDepartment's own string is "org_unit", not "department" — the literal value snapshots.go's own
	// pre-existing ViewTypeOrgUnit constant and every snapshot published before DFG-06 already use for
	// dataflow.snapshots.view_type, so this view's own published history keeps resolving under the same key.
	ViewDepartment   View = "org_unit"
	ViewCompany      View = "company"
	ViewDataCategory View = "data_category"
)

// dataflow.snapshots.view_type (DFG-07) stores one of these three strings directly — ViewTypeOrgUnit
// (snapshots.go) is this same department view's own legacy constant, kept for existing snapshots.

// Graph builds DFG-01's data flow diagram (and DFG-03's cross-border view, which shares the same
// acceptance criterion and dependency): a swimlane per View (department/company/data_category, DFG-06)
// with its activities, each activity's recipients ("send" edges) and, for ROPA-08's logged cross-border
// transfers, one "transfer" edge per transfer row — warning-coloured when a foreign recipient has no
// transfer row at all (DFG-03's own acceptance criterion: "เส้นทางโอนที่ไม่มีฐานการโอนแสดงเป็นสีเตือน"),
// mirroring ropaservice's own completeness() "transfer_basis" check for an implicit, never-logged transfer
// — plus one "disposal" node per unique retention disposal method the activity names. Computed live on
// every call, same discipline as dpia's ActivityDescription/Registry — never a stale persisted copy
// (DFG-05's layouts / DFG-07's published snapshots are a later, separate feature this pass doesn't build).
// An empty view defaults to ViewDepartment (DFG-01's own original, and still the default, grouping).
func (s *Service) Graph(ctx context.Context, legalEntityID *uuid.UUID, view View) (Graph, error) {
	if view == "" {
		view = ViewDepartment
	}
	g := Graph{GeneratedAt: time.Now().UTC()}

	seenSwimlane := map[string]bool{}
	seenRecipient := map[uuid.UUID]bool{}
	seenCountry := map[string]bool{}
	seenDisposal := map[string]bool{}

	acts, err := s.listAllActivities(ctx)
	if err != nil {
		return Graph{}, err
	}

	for _, a := range acts {
		if legalEntityID != nil && a.LegalEntityID != *legalEntityID {
			continue
		}
		g.ActivityCount++

		data, err := s.Ropa.ListActivityData(ctx, a.ID)
		if err != nil {
			return Graph{}, err
		}
		var dataCategories []string
		var primaryCategoryID uuid.UUID
		var primaryCategoryName string
		seenCategory := map[uuid.UUID]bool{}
		for _, d := range data {
			if seenCategory[d.DataCategoryID] {
				continue
			}
			seenCategory[d.DataCategoryID] = true
			cat, err := s.Org.GetMaster(ctx, "data_categories", d.DataCategoryID)
			if err != nil {
				return Graph{}, err
			}
			dataCategories = append(dataCategories, cat.NameTh)
			if primaryCategoryID == uuid.Nil {
				primaryCategoryID, primaryCategoryName = d.DataCategoryID, cat.NameTh
			}
		}

		swimlaneID, err := s.swimlaneNode(ctx, &g, seenSwimlane, view, a, primaryCategoryID, primaryCategoryName)
		if err != nil {
			return Graph{}, err
		}

		actNode := Node{
			ID: activityNodeID(a.ID), Type: "activity", Label: a.Name, ParentID: swimlaneID,
			ActivityCode: a.Code, ActivityStatus: a.Status, Role: a.Role, DataCategories: dataCategories,
		}

		recs, err := s.Ropa.ListActivityRecipients(ctx, a.ID)
		if err != nil {
			return Graph{}, err
		}
		transfers, err := s.Ropa.ListActivityTransfers(ctx, a.ID)
		if err != nil {
			return Graph{}, err
		}
		transferredRecipients := map[uuid.UUID]bool{}
		for _, tr := range transfers {
			if tr.RecipientID != nil {
				transferredRecipients[*tr.RecipientID] = true
			}
		}

		for _, r := range recs {
			party, err := s.Org.GetExternalParty(ctx, r.PartyID)
			if err != nil {
				return Graph{}, err
			}
			if !seenRecipient[r.PartyID] {
				seenRecipient[r.PartyID] = true
				node := Node{
					ID: recipientNodeID(r.PartyID), Type: "recipient", Label: party.NameTh,
					RecipientRole: r.RecipientRole, DisclosureBasis: r.DisclosureBasis, CountryCode: party.CountryCode,
				}
				if r.RecipientRole == "processor" && s.Vendor != nil && s.Agreement != nil {
					if err := s.flagMissingDPA(ctx, r.PartyID, &node); err != nil {
						return Graph{}, err
					}
				}
				if (r.RecipientRole == "controller" || r.RecipientRole == "joint_controller") && s.Agreement != nil {
					if err := s.flagMissingDSA(ctx, r.PartyID, &node); err != nil {
						return Graph{}, err
					}
				}
				g.Nodes = append(g.Nodes, node)
			}
			g.Edges = append(g.Edges, Edge{
				ID: "send:" + a.ID.String() + ":" + r.ID.String(), Source: activityNodeID(a.ID),
				Target: recipientNodeID(r.PartyID), Type: "send", Label: r.RecipientRole,
			})

			if isForeignCountry(party.CountryCode) && !transferredRecipients[r.ID] {
				if !seenCountry[party.CountryCode] {
					seenCountry[party.CountryCode] = true
					g.Nodes = append(g.Nodes, Node{ID: countryNodeID(party.CountryCode), Type: "country", Label: party.CountryCode, CountryCode: party.CountryCode})
				}
				g.Edges = append(g.Edges, Edge{
					ID: "transfer:missing:" + a.ID.String() + ":" + r.ID.String(), Source: recipientNodeID(r.PartyID),
					Target: countryNodeID(party.CountryCode), Type: "transfer", Warning: true,
				})
			}
		}

		for _, tr := range transfers {
			if !seenCountry[tr.CountryCode] {
				seenCountry[tr.CountryCode] = true
				g.Nodes = append(g.Nodes, Node{ID: countryNodeID(tr.CountryCode), Type: "country", Label: tr.CountryCode, CountryCode: tr.CountryCode})
			}
			source := activityNodeID(a.ID)
			if tr.RecipientID != nil {
				source = recipientNodeID(*tr.RecipientID)
			}
			g.Edges = append(g.Edges, Edge{
				ID: "transfer:" + tr.ID.String(), Source: source, Target: countryNodeID(tr.CountryCode),
				Type: "transfer", Label: tr.TransferBasis,
			})
		}

		rules, err := s.Ropa.ListRetentionRules(ctx, a.ID)
		if err != nil {
			return Graph{}, err
		}
		seenMethod := map[string]bool{}
		var methods []string
		for _, rule := range rules {
			if seenMethod[rule.DisposalMethod] {
				continue
			}
			seenMethod[rule.DisposalMethod] = true
			methods = append(methods, rule.DisposalMethod)
			id := disposalNodeID(a.ID, rule.DisposalMethod)
			if !seenDisposal[id] {
				seenDisposal[id] = true
				g.Nodes = append(g.Nodes, Node{ID: id, Type: "disposal", Label: rule.DisposalMethod, ParentID: activityNodeID(a.ID)})
			}
			g.Edges = append(g.Edges, Edge{ID: "destroy:" + rule.ID.String(), Source: activityNodeID(a.ID), Target: id, Type: "destroy", Label: rule.DisposalMethod})
		}
		actNode.DisposalMethods = methods

		g.Nodes = append(g.Nodes, actNode)
	}

	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	sort.Slice(g.Edges, func(i, j int) bool { return g.Edges[i].ID < g.Edges[j].ID })
	return g, nil
}

// swimlaneNode resolves (and, the first time it's seen, creates) the one swimlane node this activity's
// own ParentID should point at under the given View — department groups by org unit (DFG-01's own
// original, unchanged), company by legal entity, data_category by the activity's own primary (first)
// data category, or an "uncategorized" swimlane when it names none at all.
func (s *Service) swimlaneNode(ctx context.Context, g *Graph, seen map[string]bool, view View, a ropaservice.Activity, primaryCategoryID uuid.UUID, primaryCategoryName string) (string, error) {
	switch view {
	case ViewCompany:
		id := legalEntityNodeID(a.LegalEntityID)
		if !seen[id] {
			seen[id] = true
			le, err := s.Org.GetLegalEntity(ctx, a.LegalEntityID)
			if err != nil {
				return "", err
			}
			g.Nodes = append(g.Nodes, Node{ID: id, Type: "legal_entity", Label: le.NameTh})
		}
		return id, nil
	case ViewDataCategory:
		if primaryCategoryID == uuid.Nil {
			if !seen[uncategorizedDataCategoryNodeID] {
				seen[uncategorizedDataCategoryNodeID] = true
				g.Nodes = append(g.Nodes, Node{ID: uncategorizedDataCategoryNodeID, Type: "data_category", Label: "ไม่ระบุหมวดข้อมูล"})
			}
			return uncategorizedDataCategoryNodeID, nil
		}
		id := dataCategoryNodeID(primaryCategoryID)
		if !seen[id] {
			seen[id] = true
			g.Nodes = append(g.Nodes, Node{ID: id, Type: "data_category", Label: primaryCategoryName})
		}
		return id, nil
	default: // ViewDepartment
		id := orgUnitNodeID(a.OrgUnitID)
		if !seen[id] {
			seen[id] = true
			unit, err := s.Org.GetOrgUnit(ctx, a.OrgUnitID)
			if err != nil {
				return "", err
			}
			g.Nodes = append(g.Nodes, Node{ID: id, Type: "org_unit", Label: unit.NameTh})
		}
		return id, nil
	}
}

// flagMissingDPA is DFG-04's own acceptance criterion: a processor-role recipient who is tracked as a
// VEN-01 vendor but has no agreement.agreements row of type "dpa" is flagged on the node itself, the same
// warning-colour convention DFG-03's own transfer edges already use — a processor never onboarded as a
// vendor at all is left alone, since there's no agreement list to check or link to.
func (s *Service) flagMissingDPA(ctx context.Context, partyID uuid.UUID, node *Node) error {
	v, err := s.Vendor.GetVendorByPartyID(ctx, partyID)
	if errors.Is(err, vendorservice.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	node.VendorID = &v.ID
	agreements, _, err := s.Agreement.ListAgreements(ctx, agreementservice.AgreementFilter{AgreementType: "dpa", VendorID: &v.ID})
	if err != nil {
		return err
	}
	for _, a := range agreements {
		node.AgreementIDs = append(node.AgreementIDs, a.ID)
	}
	node.Warning = len(agreements) == 0
	return nil
}

// flagMissingDSA is DSA-12's own acceptance criterion: a recipient role of "controller" or
// "joint_controller" — another controller this activity shares data with, ม.39's own cross-controller
// sharing case — is flagged when it has no agreement.agreements row of type "dsa" naming it as the
// counterparty. Unlike flagMissingDPA there is no vendor gate first: a receiving controller is never a
// VEN-01 vendor (that relationship is processor-only), so the external party id itself is the only check.
func (s *Service) flagMissingDSA(ctx context.Context, partyID uuid.UUID, node *Node) error {
	agreements, _, err := s.Agreement.ListAgreements(ctx, agreementservice.AgreementFilter{AgreementType: "dsa", CounterpartyID: &partyID})
	if err != nil {
		return err
	}
	for _, a := range agreements {
		node.AgreementIDs = append(node.AgreementIDs, a.ID)
	}
	if len(agreements) == 0 {
		node.Warning = true
	}
	return nil
}

// listAllActivities drains ropaservice.ListActivities' own cursor pagination (activityPageSize=50) rather
// than adding a second, unpaginated ropa query just for this one caller — the acceptance criterion's own
// "50 activities" is one or two pages.
func (s *Service) listAllActivities(ctx context.Context) ([]ropaservice.Activity, error) {
	var out []ropaservice.Activity
	var after *ropaservice.ActivityCursor
	for {
		rows, next, err := s.Ropa.ListActivities(ctx, ropaservice.ActivityFilter{After: after})
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if next == nil {
			break
		}
		after = next
	}
	return out, nil
}
