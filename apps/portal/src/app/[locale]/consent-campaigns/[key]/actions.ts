"use server";

import { randomUUID } from "node:crypto";
import { respondToCampaign } from "@/lib/campaigns";
import type { CampaignView } from "@/lib/campaigns";

export type RespondState = {
  idem: string;
  receipt?: string;
  error?: string;
};

// CON-24: the recipient grants/refuses any offered purpose in one call — untouched purposes are left
// undecided, the same "partial decisions are allowed here" rule the preference centre's own save action uses.
export async function respondAction(key: string, locale: string, token: string, view: CampaignView, prev: RespondState, form: FormData): Promise<RespondState> {
  const decisions: { purpose_code: string; purpose_version_no: number; decision: "CONSENTED" | "NOT_CONSENTED" }[] = [];
  for (const p of view.purposes) {
    const choice = form.get(`decision:${p.code}`);
    if (choice !== "yes" && choice !== "no") continue;
    decisions.push({ purpose_code: p.code, purpose_version_no: p.version_no, decision: choice === "yes" ? "CONSENTED" : "NOT_CONSENTED" });
  }
  if (decisions.length === 0) return { idem: prev.idem, error: "no_change" };
  const res = await respondToCampaign(key, prev.idem, token, decisions, locale);
  if (res.ok) return { idem: randomUUID(), receipt: res.data.receipt_no };
  return { idem: randomUUID(), error: res.problem.code };
}
