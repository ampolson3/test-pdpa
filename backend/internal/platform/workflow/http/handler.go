// Package workflowhttp holds the PLT-05 workflow endpoints. Types in workflow.gen.go are generated from
// api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package workflowhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/workflow"
)

type Strict struct {
	svc *workflow.Service
}

func NewStrict(svc *workflow.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) PlatformListWorkflowDefinitions(ctx context.Context, _ PlatformListWorkflowDefinitionsRequestObject) (PlatformListWorkflowDefinitionsResponseObject, error) {
	list, err := h.svc.ListDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	resp := PlatformListWorkflowDefinitions200JSONResponse{Data: make([]WorkflowDefinition, 0, len(list))}
	for _, d := range list {
		w, err := definitionWire(ctx, d)
		if err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

func (h *Strict) PlatformCreateWorkflowDefinition(ctx context.Context, req PlatformCreateWorkflowDefinitionRequestObject) (PlatformCreateWorkflowDefinitionResponseObject, error) {
	in, err := definitionInput(*req.Body)
	if err != nil {
		return nil, problem(err)
	}
	d, err := h.svc.CreateDefinition(ctx, in)
	if err != nil {
		return nil, problem(err)
	}
	w, err := definitionWire(ctx, d)
	if err != nil {
		return nil, err
	}
	return PlatformCreateWorkflowDefinition201JSONResponse{Body: w, Headers: PlatformCreateWorkflowDefinition201ResponseHeaders{ETag: etag(d.RowVersion)}}, nil
}

func (h *Strict) PlatformGetWorkflowDefinition(ctx context.Context, req PlatformGetWorkflowDefinitionRequestObject) (PlatformGetWorkflowDefinitionResponseObject, error) {
	d, err := h.svc.GetDefinition(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	w, err := definitionWire(ctx, d)
	if err != nil {
		return nil, err
	}
	return PlatformGetWorkflowDefinition200JSONResponse{Body: w, Headers: PlatformGetWorkflowDefinition200ResponseHeaders{ETag: etag(d.RowVersion)}}, nil
}

func (h *Strict) PlatformSaveWorkflowVersion(ctx context.Context, req PlatformSaveWorkflowVersionRequestObject) (PlatformSaveWorkflowVersionResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in, err := definitionInput(*req.Body)
	if err != nil {
		return nil, problem(err)
	}
	d, err := h.svc.SaveVersion(ctx, req.Id, v, in)
	if err != nil {
		return nil, problem(err)
	}
	w, err := definitionWire(ctx, d)
	if err != nil {
		return nil, err
	}
	return PlatformSaveWorkflowVersion201JSONResponse{Body: w, Headers: PlatformSaveWorkflowVersion201ResponseHeaders{ETag: etag(d.RowVersion)}}, nil
}

func (h *Strict) PlatformSearchAssignableGroups(ctx context.Context, req PlatformSearchAssignableGroupsRequestObject) (PlatformSearchAssignableGroupsResponseObject, error) {
	q := ""
	if req.Params.Q != nil {
		q = *req.Params.Q
	}
	groups, err := iamservice.SearchGroups(ctx, q)
	if err != nil {
		return nil, err
	}
	resp := PlatformSearchAssignableGroups200JSONResponse{Data: make([]NamedRef, 0, len(groups))}
	for _, g := range groups {
		resp.Data = append(resp.Data, NamedRef{Id: g.ID, Name: g.DisplayName})
	}
	return resp, nil
}

func (h *Strict) PlatformGetWorkflowInstance(ctx context.Context, req PlatformGetWorkflowInstanceRequestObject) (PlatformGetWorkflowInstanceResponseObject, error) {
	v, err := h.svc.Get(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	w, err := instanceWire(v)
	if err != nil {
		return nil, err
	}
	return PlatformGetWorkflowInstance200JSONResponse{Body: w, Headers: PlatformGetWorkflowInstance200ResponseHeaders{ETag: etag(v.RowVersion)}}, nil
}

func (h *Strict) PlatformTransitionWorkflow(ctx context.Context, req PlatformTransitionWorkflowRequestObject) (PlatformTransitionWorkflowResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	comment := ""
	if req.Body.Comment != nil {
		comment = *req.Body.Comment
	}
	if _, err := h.svc.Transition(ctx, req.Id, ver, req.Body.To, comment); err != nil {
		return nil, problem(err)
	}
	v, err := h.svc.Get(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	w, err := instanceWire(v)
	if err != nil {
		return nil, err
	}
	return PlatformTransitionWorkflow200JSONResponse{Body: w, Headers: PlatformTransitionWorkflow200ResponseHeaders{ETag: etag(v.RowVersion)}}, nil
}

func (h *Strict) PlatformListMyTasks(ctx context.Context, _ PlatformListMyTasksRequestObject) (PlatformListMyTasksResponseObject, error) {
	items, err := h.svc.MyTasks(ctx)
	if err != nil {
		return nil, problem(err)
	}
	resp := PlatformListMyTasks200JSONResponse{Data: make([]MyTask, 0, len(items))}
	for _, it := range items {
		m := taskMap(it.Task)
		m["entity_type"], m["entity_id"], m["workflow_name"], m["workflow_code"] = it.EntityType, it.EntityID, it.WorkflowName, it.WorkflowCode
		m["state_label"], m["sla_status"] = it.StateLabel, it.SLAStatus
		if it.SLADueAt != nil {
			m["sla_due_at"] = it.SLADueAt.UTC()
		}
		var w MyTask
		if err := convert(m, &w); err != nil {
			return nil, err
		}
		resp.Data = append(resp.Data, w)
	}
	return resp, nil
}

func (h *Strict) PlatformUpdateWorkflowTask(ctx context.Context, req PlatformUpdateWorkflowTaskRequestObject) (PlatformUpdateWorkflowTaskResponseObject, error) {
	ver, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	c := workflow.TaskChange{AssigneeUserID: req.Body.AssigneeUserId, AssigneeGroupID: req.Body.AssigneeGroupId}
	if req.Body.Status != nil {
		c.Status = string(*req.Body.Status)
	}
	t, err := h.svc.UpdateTask(ctx, req.Id, ver, c)
	if err != nil {
		return nil, problem(err)
	}
	var w WorkflowTask
	if err := convert(taskMap(t), &w); err != nil {
		return nil, err
	}
	return PlatformUpdateWorkflowTask200JSONResponse{Body: w, Headers: PlatformUpdateWorkflowTask200ResponseHeaders{ETag: etag(t.RowVersion)}}, nil
}

// ---- wire conversion (the JSON shapes of the engine's definition and the contract are the same) ----

func convert(in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func definitionInput(b WorkflowDefinitionInput) (workflow.DefinitionInput, error) {
	var d workflow.Definition
	if err := convert(b.Definition, &d); err != nil {
		return workflow.DefinitionInput{}, fmt.Errorf("%w: %v", workflow.ErrInvalidDefinition, err)
	}
	return workflow.DefinitionInput{Code: b.Code, Name: b.Name, EntityType: b.EntityType, Definition: d}, nil
}

func definitionWire(ctx context.Context, d workflow.DefinitionRecord) (WorkflowDefinition, error) {
	users, groups := d.Definition.References()
	names := map[string]string{}
	un, err := iamservice.Names(ctx, users)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	gn, err := iamservice.GroupNames(ctx, groups)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	for id, n := range un {
		names[id.String()] = n
	}
	for id, n := range gn {
		names[id.String()] = n
	}
	var w WorkflowDefinition
	err = convert(map[string]any{"id": d.ID, "global": d.Global, "code": d.Code, "name": d.Name, "entity_type": d.EntityType, "version": d.Version,
		"definition": d.Definition, "active": d.Active, "names": names, "row_version": d.RowVersion, "updated_at": d.UpdatedAt.UTC()}, &w)
	return w, err
}

func taskMap(t workflow.Task) map[string]any {
	m := map[string]any{"id": t.ID, "instance_id": t.InstanceID, "state": t.State, "title": t.Title, "status": t.Status,
		"row_version": t.RowVersion, "created_at": t.CreatedAt.UTC()}
	if t.AssigneeUserID != nil {
		m["assignee_user_id"], m["assignee_name"] = t.AssigneeUserID, t.AssigneeName
	}
	if t.AssigneeGroupID != nil {
		m["assignee_group_id"], m["group_name"] = t.AssigneeGroupID, t.GroupName
	}
	putTime(m, "due_at", t.DueAt)
	putTime(m, "completed_at", t.CompletedAt)
	if t.Outcome != "" {
		m["outcome"] = t.Outcome
	}
	if t.Comment != "" {
		m["comment"] = t.Comment
	}
	return m
}

func instanceWire(v workflow.InstanceView) (WorkflowInstance, error) {
	tasks := make([]map[string]any, 0, len(v.Tasks))
	for _, t := range v.Tasks {
		tasks = append(tasks, taskMap(t))
	}
	timers := make([]map[string]any, 0, len(v.Timers))
	for _, t := range v.Timers {
		rs := make([]map[string]any, 0, len(t.Reminders))
		for _, r := range t.Reminders {
			rs = append(rs, map[string]any{"at": r.At.UTC(), "sent": r.Sent})
		}
		m := map[string]any{"code": t.Code, "mode": t.Mode, "started_at": t.StartedAt.UTC(), "due_at": t.DueAt.UTC(), "status": t.Status, "paused": t.Paused, "reminders": rs}
		putTime(m, "stopped_at", t.StoppedAt)
		timers = append(timers, m)
	}
	history := make([]map[string]any, 0, len(v.History))
	for _, e := range v.History {
		m := map[string]any{"at": e.At.UTC(), "action": e.Action}
		if e.ActorName != "" {
			m["actor_name"] = e.ActorName
		}
		if e.Before != nil {
			m["before"] = e.Before
		}
		if e.After != nil {
			m["after"] = e.After
		}
		history = append(history, m)
	}
	transitions := v.Transitions
	if transitions == nil {
		transitions = []workflow.Transition{}
	}
	m := map[string]any{"id": v.ID, "entity_type": v.EntityType, "entity_id": v.EntityID, "state": v.State, "started_at": v.StartedAt.UTC(),
		"sla_status": v.SLAStatus, "row_version": v.RowVersion, "tasks": tasks, "timers": timers, "history": history, "transitions": transitions,
		"workflow": map[string]any{"id": v.Workflow.ID, "code": v.Workflow.Code, "name": v.Workflow.Name, "version": v.Workflow.Version, "definition": v.Workflow.Definition}}
	putTime(m, "completed_at", v.CompletedAt)
	var w WorkflowInstance
	return w, convert(m, &w)
}

func putTime(m map[string]any, k string, t *time.Time) {
	if t != nil {
		m[k] = t.UTC()
	}
}

func etag(v int32) *string {
	s := fmt.Sprintf("%q", strconv.Itoa(int(v)))
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

func problem(err error) error {
	switch {
	case errors.Is(err, workflow.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, workflow.ErrForbidden):
		return httpx.AuthzDenied()
	case errors.Is(err, workflow.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, workflow.ErrInvalidTransition):
		return httpx.Problem{Status: http.StatusConflict, Code: "workflow.invalid_transition", Title: "Invalid state transition"}
	case errors.Is(err, workflow.ErrInvalidDefinition):
		return httpx.UnprocessableEntity("workflow.invalid_definition", err.Error())
	case errors.Is(err, workflow.ErrInvalidRequest):
		return httpx.UnprocessableEntity("workflow.invalid_request", err.Error())
	}
	return err
}
