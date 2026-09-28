"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { createApiClient, useSaveScreeningRules, useScreeningRules } from "@pdpa/api-client";

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

export function DpiaSettingsContent() {
  const t = useTranslations("dpia");
  const canRead = usePermission("assessment.template.read");
  const canUpdate = usePermission("assessment.template.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const rules = useScreeningRules(client);
  const save = useSaveScreeningRules(client);

  const [minFactors, setMinFactors] = useState(2);
  const [minScore, setMinScore] = useState("");
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    if (rules.data) {
      setMinFactors(rules.data.min_factors);
      setMinScore(rules.data.min_score != null ? String(rules.data.min_score) : "");
    }
  }, [rules.data]);

  if (!canRead) return <main className="mx-auto max-w-3xl p-8 text-slate-600">{t("forbidden")}</main>;
  if (rules.isPending) return <main className="mx-auto max-w-3xl p-8 text-slate-500">{t("loading")}</main>;
  if (rules.isError) return <main className="mx-auto max-w-3xl p-8 text-red-700">{t("loadError")}</main>;

  return (
    <main className="mx-auto max-w-3xl space-y-4 p-8 text-sm">
      <h1 className="text-xl font-semibold">{t("settings.title")}</h1>
      <p className="text-slate-500">{t("settings.intro")}</p>
      <div className="space-y-3 rounded-md border border-slate-200 bg-white p-4">
        <label className="block">
          {t("settings.minFactors")}
          <input
            type="number"
            min={1}
            max={6}
            className={INPUT}
            value={minFactors}
            disabled={!canUpdate}
            onChange={(e) => setMinFactors(Number(e.target.value))}
          />
        </label>
        <label className="block">
          {t("settings.minScore")}
          <input
            type="number"
            min={0}
            className={INPUT}
            value={minScore}
            disabled={!canUpdate}
            onChange={(e) => setMinScore(e.target.value)}
          />
        </label>
        {save.isError && <p className="text-red-700">{t("settings.saveError", { detail: detail(save.error) })}</p>}
        {saved && !save.isPending && <p className="text-green-700">{t("settings.saved")}</p>}
        {canUpdate && (
          <Button
            onClick={() =>
              save.mutate(
                { min_factors: minFactors, min_score: minScore === "" ? null : Number(minScore) },
                { onSuccess: () => setSaved(true) },
              )
            }
            disabled={save.isPending}
          >
            {t("settings.save")}
          </Button>
        )}
      </div>
    </main>
  );
}
