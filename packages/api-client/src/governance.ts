import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type RetentionSchedule = components["schemas"]["RetentionSchedule"];
export type RetentionScheduleInput = components["schemas"]["RetentionScheduleInput"];
export type RetentionScheduleStatus = components["schemas"]["RetentionScheduleStatus"];
export type DisposalJob = components["schemas"]["DisposalJob"];
export type DisposalJobStatus = components["schemas"]["DisposalJobStatus"];
export type DisposalJobCompleteInput = components["schemas"]["DisposalJobCompleteInput"];
export type GovPolicy = components["schemas"]["GovPolicy"];
export type GovPolicyInput = components["schemas"]["GovPolicyInput"];
export type GovPolicyAttestation = components["schemas"]["GovPolicyAttestation"];
export type GovMissingAttestation = components["schemas"]["GovMissingAttestation"];
export type GovRegulatorLetter = components["schemas"]["GovRegulatorLetter"];
export type GovRegulatorLetterInput = components["schemas"]["GovRegulatorLetterInput"];
export type GovRegulatorLetterStatus = components["schemas"]["GovRegulatorLetterStatus"];
export type GovRegulatorLetterDirection = components["schemas"]["GovRegulatorLetterDirection"];
export type GovCourse = components["schemas"]["GovCourse"];
export type GovCourseInput = components["schemas"]["GovCourseInput"];
export type GovCourseStatus = components["schemas"]["GovCourseStatus"];
export type GovCourseContentType = components["schemas"]["GovCourseContentType"];
export type GovTrainingAssignment = components["schemas"]["GovTrainingAssignment"];
export type GovTrainingAssignmentInput = components["schemas"]["GovTrainingAssignmentInput"];
export type GovTrainingAssignmentTargetType = components["schemas"]["GovTrainingAssignmentTargetType"];
export type GovTrainingAttempt = components["schemas"]["GovTrainingAttempt"];
export type GovAudit = components["schemas"]["GovAudit"];
export type GovAuditSectionScore = components["schemas"]["GovAuditSectionScore"];
export type GovAuditFinding = components["schemas"]["GovAuditFinding"];
export type GovAuditFindingInput = components["schemas"]["GovAuditFindingInput"];
export type GovAuditFindingSeverity = components["schemas"]["GovAuditFindingSeverity"];
export type GovAuditFindingStatus = components["schemas"]["GovAuditFindingStatus"];

const schedulesKey = ["gov", "retention-schedules"] as const;
const jobsKey = ["gov", "disposal-jobs"] as const;
const policiesKey = ["gov", "policies"] as const;
const attestationsKey = ["gov", "policy-attestations"] as const;
const missingAttestationsKey = ["gov", "missing-attestations"] as const;
const regulatorLettersKey = ["gov", "regulator-letters"] as const;
const coursesKey = ["gov", "courses"] as const;
const trainingAssignmentsKey = ["gov", "training-assignments"] as const;
const myTrainingAssignmentsKey = ["gov", "my-training-assignments"] as const;
const myTrainingAttemptsKey = ["gov", "my-training-attempts"] as const;
const trainingReportKey = ["gov", "training-report"] as const;
const auditsKey = ["gov", "audits"] as const;
const auditFindingsKey = ["gov", "audit-findings"] as const;

/** GET /admin/v1/governance/retention-schedules (DPX-05), newest first. */
export function useRetentionSchedules(client: ApiClient, filter: { assetId?: string; status?: RetentionScheduleStatus } = {}) {
  return useInfiniteQuery({
    queryKey: [...schedulesKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/governance/retention-schedules", {
        params: { query: { asset_id: filter.assetId, status: filter.status, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** Create a retention schedule against a RoPA retention rule and asset. */
export function useCreateRetentionSchedule(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: RetentionScheduleInput) => {
      const { data, error } = await client.POST("/admin/v1/governance/retention-schedules", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: schedulesKey }),
  });
}

/** PATCH /admin/v1/governance/retention-schedules/{id} — pause / resume / retire. */
export function useSetRetentionScheduleStatus(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; status: RetentionScheduleStatus }) => {
      const { data, error } = await client.PATCH("/admin/v1/governance/retention-schedules/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: { status: v.status },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: schedulesKey }),
  });
}

/** GET /admin/v1/governance/disposal-jobs — created automatically by the daily due-date sweep. */
export function useDisposalJobs(client: ApiClient, filter: { status?: DisposalJobStatus } = {}) {
  return useInfiniteQuery({
    queryKey: [...jobsKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/governance/disposal-jobs", {
        params: { query: { status: filter.status, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** POST /admin/v1/governance/disposal-jobs/{id}/approve — refused if the asset is under an active legal hold. */
export function useApproveDisposalJob(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; assigneeUserId?: string }) => {
      const { data, error } = await client.POST("/admin/v1/governance/disposal-jobs/{id}/approve", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: { assignee_user_id: v.assigneeUserId },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: jobsKey }),
  });
}

/** POST /admin/v1/governance/disposal-jobs/{id}/start — IT begins the actual disposal work. */
export function useStartDisposalJob(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number }) => {
      const { data, error } = await client.POST("/admin/v1/governance/disposal-jobs/{id}/start", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: jobsKey }),
  });
}

/** POST /admin/v1/governance/disposal-jobs/{id}/complete — the acceptance criterion's gate: refused without evidence. */
export function useCompleteDisposalJob(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; input: DisposalJobCompleteInput }) => {
      const { data, error } = await client.POST("/admin/v1/governance/disposal-jobs/{id}/complete", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: jobsKey }),
  });
}

/** POST /admin/v1/governance/disposal-jobs/{id}/release-hold — once the legal hold has been lifted. */
export function useReleaseDisposalJobHold(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number }) => {
      const { data, error } = await client.POST("/admin/v1/governance/disposal-jobs/{id}/release-hold", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: jobsKey }),
  });
}

/** GET /admin/v1/governance/policies (DPX-02) — every tracked policy, newest published first. */
export function usePolicies(client: ApiClient) {
  return useQuery({
    queryKey: policiesKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/policies", {});
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/governance/policies — track an already-published PLT-16 document's current version. */
export function usePublishPolicy(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: GovPolicyInput) => {
      const { data, error } = await client.POST("/admin/v1/governance/policies", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: policiesKey }),
  });
}

/** POST /admin/v1/governance/policies/{id}/attest — the signed-in user acknowledges the current version. */
export function useAttestPolicy(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (policyId: string) => {
      const { data, error } = await client.POST("/admin/v1/governance/policies/{id}/attest", { params: { path: { id: policyId } } });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, policyId) => {
      qc.invalidateQueries({ queryKey: [...attestationsKey, policyId] });
      qc.invalidateQueries({ queryKey: [...missingAttestationsKey, policyId] });
    },
  });
}

/** GET /admin/v1/governance/policies/{id}/attestations — every attestation recorded, oldest first. */
export function usePolicyAttestations(client: ApiClient, policyId: string) {
  return useQuery({
    queryKey: [...attestationsKey, policyId],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/policies/{id}/attestations", { params: { path: { id: policyId } } });
      if (error) throw error;
      return data.data;
    },
    enabled: !!policyId,
  });
}

/** GET /admin/v1/governance/policies/{id}/missing-attestations — the acceptance criterion itself. */
export function useMissingAttestations(client: ApiClient, policyId: string) {
  return useQuery({
    queryKey: [...missingAttestationsKey, policyId],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/policies/{id}/missing-attestations", { params: { path: { id: policyId } } });
      if (error) throw error;
      return data.data;
    },
    enabled: !!policyId,
  });
}

/** GET /admin/v1/governance/regulator-letters (DPX-08) — soonest-due first. */
export function useRegulatorLetters(client: ApiClient, filter: { status?: GovRegulatorLetterStatus; direction?: GovRegulatorLetterDirection } = {}) {
  return useQuery({
    queryKey: [...regulatorLettersKey, filter],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/regulator-letters", {
        params: { query: { status: filter.status, direction: filter.direction } },
      });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/governance/regulator-letters — a real due_at schedules the single due-date reminder. */
export function useCreateRegulatorLetter(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: GovRegulatorLetterInput) => {
      const { data, error } = await client.POST("/admin/v1/governance/regulator-letters", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: regulatorLettersKey }),
  });
}

/** PUT /admin/v1/governance/regulator-letters/{id} — a changed due_at reschedules the reminder. */
export function useUpdateRegulatorLetter(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; input: GovRegulatorLetterInput }) => {
      const { data, error } = await client.PUT("/admin/v1/governance/regulator-letters/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: regulatorLettersKey }),
  });
}

/** PATCH /admin/v1/governance/regulator-letters/{id}/status — open -> responded -> closed. */
export function useSetRegulatorLetterStatus(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; status: GovRegulatorLetterStatus }) => {
      const { data, error } = await client.PATCH("/admin/v1/governance/regulator-letters/{id}/status", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: { status: v.status },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: regulatorLettersKey }),
  });
}

/** DELETE /admin/v1/governance/regulator-letters/{id} — removes the letter record (not its file). */
export function useDeleteRegulatorLetter(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await client.DELETE("/admin/v1/governance/regulator-letters/{id}", { params: { path: { id } } });
      if (error) throw error;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: regulatorLettersKey }),
  });
}

/** GET /admin/v1/governance/courses (DPX-01) — PDPA awareness courses, newest first. */
export function useCourses(client: ApiClient, status?: GovCourseStatus) {
  return useQuery({
    queryKey: [...coursesKey, status],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/courses", { params: { query: { status } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/governance/courses — start a new draft course. */
export function useCreateCourse(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: GovCourseInput) => {
      const { data, error } = await client.POST("/admin/v1/governance/courses", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: coursesKey }),
  });
}

/** PATCH /admin/v1/governance/courses/{id}/status — draft -> published -> retired. */
export function useSetCourseStatus(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; status: GovCourseStatus }) => {
      const { data, error } = await client.PATCH("/admin/v1/governance/courses/{id}/status", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: { status: v.status },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: coursesKey }),
  });
}

/** GET /admin/v1/governance/courses/{id}/report — DPX-01's own acceptance criterion: results per individual. */
export function useTrainingReport(client: ApiClient, courseId: string) {
  return useQuery({
    queryKey: [...trainingReportKey, courseId],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/courses/{id}/report", { params: { path: { id: courseId } } });
      if (error) throw error;
      return data.data;
    },
    enabled: !!courseId,
  });
}

/** GET /admin/v1/governance/training-assignments — every assignment, optionally narrowed by course. */
export function useTrainingAssignments(client: ApiClient, courseId?: string) {
  return useQuery({
    queryKey: [...trainingAssignmentsKey, courseId],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/training-assignments", { params: { query: { course_id: courseId } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/governance/training-assignments — assign a published course to all/an org unit/a group/a user. */
export function useAssignTraining(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: GovTrainingAssignmentInput) => {
      const { data, error } = await client.POST("/admin/v1/governance/training-assignments", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: trainingAssignmentsKey }),
  });
}

/** GET /admin/v1/governance/my-training-assignments — what currently applies to the signed-in employee. */
export function useMyTrainingAssignments(client: ApiClient) {
  return useQuery({
    queryKey: myTrainingAssignmentsKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/my-training-assignments", {});
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/governance/courses/{id}/attempts — the employee begins a published course. */
export function useStartAttempt(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { courseId: string; assignmentId?: string }) => {
      const { data, error } = await client.POST("/admin/v1/governance/courses/{id}/attempts", {
        params: { path: { id: v.courseId } },
        body: { assignment_id: v.assignmentId },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: myTrainingAttemptsKey }),
  });
}

/** POST /admin/v1/governance/training-attempts/{id}/submit — the acceptance criterion: complete the quiz (if any). */
export function useSubmitAttempt(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; answers?: Record<string, unknown> }) => {
      const { data, error } = await client.POST("/admin/v1/governance/training-attempts/{id}/submit", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: { answers: v.answers },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: myTrainingAttemptsKey });
      qc.invalidateQueries({ queryKey: trainingReportKey });
    },
  });
}

/** GET /admin/v1/governance/my-training-attempts — the signed-in employee's own past attempts. */
export function useMyTrainingAttempts(client: ApiClient) {
  return useQuery({
    queryKey: myTrainingAttemptsKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/my-training-attempts", {});
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/governance/audits (DPX-03) — organization-wide/per-subsidiary PDPA-readiness audits. */
export function useAudits(client: ApiClient, legalEntityId?: string) {
  return useQuery({
    queryKey: [...auditsKey, legalEntityId],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/audits", { params: { query: { legal_entity_id: legalEntityId } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/governance/audits — the acceptance criterion: a score per dimension and an opened remediation plan. */
export function useRunAudit(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: { legal_entity_id: string; period: string; template_code: string; answers: Record<string, unknown> }) => {
      const { data, error } = await client.POST("/admin/v1/governance/audits", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: auditsKey }),
  });
}

/** GET /admin/v1/governance/audits/{id} — one audit with its own current scores and findings. */
export function useAudit(client: ApiClient, id: string) {
  return useQuery({
    queryKey: [...auditsKey, "detail", id],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/audits/{id}", { params: { path: { id } } });
      if (error) throw error;
      return data;
    },
    enabled: !!id,
  });
}

/** DELETE /admin/v1/governance/audits/{id} — removes the audit's own wrapper row. */
export function useDeleteAudit(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await client.DELETE("/admin/v1/governance/audits/{id}", { params: { path: { id } } });
      if (error) throw error;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: auditsKey }),
  });
}

/** GET /admin/v1/governance/audits/{id}/findings — the audit's own remediation plan. */
export function useAuditFindings(client: ApiClient, auditId: string) {
  return useQuery({
    queryKey: [...auditFindingsKey, auditId],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/audits/{id}/findings", { params: { path: { id: auditId } } });
      if (error) throw error;
      return data.data;
    },
    enabled: !!auditId,
  });
}

/** POST /admin/v1/governance/audits/{id}/findings — record a finding by hand. */
export function useAddAuditFinding(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { auditId: string; input: GovAuditFindingInput }) => {
      const { data, error } = await client.POST("/admin/v1/governance/audits/{id}/findings", { params: { path: { id: v.auditId } }, body: v.input });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: [...auditFindingsKey, v.auditId] }),
  });
}

/** PATCH /admin/v1/governance/audit-findings/{id}/status — open -> in_progress -> done/accepted. */
export function useSetAuditFindingStatus(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; status: GovAuditFindingStatus }) => {
      const { data, error } = await client.PATCH("/admin/v1/governance/audit-findings/{id}/status", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: { status: v.status },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: auditFindingsKey }),
  });
}

/** POST /admin/v1/governance/audit-findings/{id}/evidence (DPX-04) — the precondition SetAuditFindingStatus
 *  checks before allowing done/accepted. */
export function useAttachFindingEvidence(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; evidenceFileId: string }) => {
      const { data, error } = await client.POST("/admin/v1/governance/audit-findings/{id}/evidence", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: { evidence_file_id: v.evidenceFileId },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: auditFindingsKey }),
  });
}

/** POST /admin/v1/governance/audit-findings/{id}/assign-task (DPX-04) — open one dpo.tasks job for this
 *  finding (assignee/due date/priority); idempotent, a finding that already has a task is returned
 *  unchanged. */
export function useAssignFindingTask(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; assigneeUserId?: string; dueAt?: string; priority?: string }) => {
      const { data, error } = await client.POST("/admin/v1/governance/audit-findings/{id}/assign-task", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: { assignee_user_id: v.assigneeUserId, due_at: v.dueAt, priority: v.priority as never },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: auditFindingsKey }),
  });
}
