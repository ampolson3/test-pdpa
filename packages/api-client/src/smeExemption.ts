import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

// ROPA-20: SME exemption screening (ประกาศ สคส. ยกเว้นกิจการขนาดเล็ก พ.ศ. 2565/2567).
export type RopaSmeExemptionCheck = components["schemas"]["RopaSmeExemptionCheck"];
export type RopaSmeExemptionCheckInput = components["schemas"]["RopaSmeExemptionCheckInput"];
export type RopaSmeExemptionResult = components["schemas"]["RopaSmeExemptionResult"];

const smeExemptionChecksKey = (legalEntityId: string) => ["ropa", "sme-exemption-checks", legalEntityId];

/** GET /admin/v1/ropa/sme-exemption-checks — past screening rounds for a legal entity, newest first. */
export function useSmeExemptionChecks(client: ApiClient, legalEntityId: string) {
  return useQuery({
    queryKey: smeExemptionChecksKey(legalEntityId),
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/sme-exemption-checks", { params: { query: { legal_entity_id: legalEntityId } } });
      if (error) throw error;
      return data.data;
    },
    enabled: !!legalEntityId,
  });
}

/** POST /admin/v1/ropa/sme-exemption-checks — the acceptance criterion itself: answer the seeded screening
 *  form and get back the result + the basis citing the relevant notice(s). */
export function useAssessSmeExemption(client: ApiClient, legalEntityId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: RopaSmeExemptionCheckInput) => {
      const { data, error } = await client.POST("/admin/v1/ropa/sme-exemption-checks", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: smeExemptionChecksKey(legalEntityId) }),
  });
}
