"use client";

import { useEffect, useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button, FileDropzone, ProgressBar } from "@pdpa/ui";
import { createApiClient, importErrorsHref, uploadFile, useImport, useImportMutations, type ImportJob, type UploadProblem } from "@pdpa/api-client";

const BFF = "/api/bff";

/**
 * The bulk-import wizard (PLT-14): upload → map columns → check every row (dry run, with an error
 * report) → confirm → result. Any module mounts it with its import type; `onDone` runs after a
 * successful import so the page can refresh its data.
 */
export function ImportWizard({ importType, onDone }: { importType: string; onDone?: () => void }) {
  const t = useTranslations("import");
  const locale = useLocale();
  const client = useMemo(() => createApiClient(BFF), []);
  const [jobId, setJobId] = useState<string>();
  const [upload, setUpload] = useState<{ progress: number; problem?: UploadProblem }>();
  const job = useImport(client, jobId).data;
  const m = useImportMutations(client);
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [remap, setRemap] = useState(false);

  useEffect(() => {
    if (job && Object.keys(mapping).length === 0 && Object.keys(job.suggested).length > 0) setMapping(job.mapping && Object.keys(job.mapping).length ? job.mapping : job.suggested);
  }, [job, mapping]);
  useEffect(() => {
    if (job?.status === "done") onDone?.();
  }, [job?.status, onDone]);

  const start = (files: File[]) => {
    const f = files[0];
    if (!f) return;
    setUpload({ progress: 0 });
    uploadFile(BFF, f, (p) => setUpload({ progress: p }))
      .then((stored) => m.create.mutateAsync({ importType, fileId: stored.id }))
      .then((j) => { setJobId(j.id); setUpload(undefined); })
      .catch((problem: UploadProblem) => setUpload({ progress: 0, problem }));
  };
  const reset = () => { setJobId(undefined); setMapping({}); setRemap(false); setUpload(undefined); };

  if (!job) {
    return (
      <div className="space-y-2">
        <FileDropzone onFiles={start} accept=".csv,.xlsx" inputLabel={t("choose")} prompt={<span>{t("dropHere")}<br /><span className="text-xs text-slate-500">{t("formats")}</span></span>} />
        {upload && !upload.problem && <ProgressBar value={upload.progress} label={t("uploading")} />}
        {upload?.problem && <p className="text-sm text-red-700">{t("uploadFailed")}</p>}
      </div>
    );
  }

  const label = (c: ImportJob["columns"][number]) => c.label[locale] ?? c.label.th ?? c.key;
  const step = job.status === "queued" && job.headers.length === 0 ? "scanning" : remap || job.status === "queued" ? "mapping" : job.status;

  return (
    <div className="space-y-3 rounded-md border border-slate-200 bg-white p-4 text-sm">
      {step === "scanning" && <Waiting text={t("scanning")} />}
      {step === "mapping" && (
        <>
          <p className="text-slate-600">{t("mapHint")}</p>
          <table className="w-full">
            <tbody>
              {job.columns.map((c) => (
                <tr key={c.key}>
                  <td className="py-1 pr-3">{label(c)}{c.required && <span className="text-red-600"> *</span>}</td>
                  <td className="py-1">
                    <select aria-label={label(c)} className="w-full rounded-md border border-slate-300 bg-white px-2 py-1" value={mapping[c.key] ?? ""}
                      onChange={(e) => setMapping({ ...mapping, [c.key]: e.target.value })}>
                      <option value="">{t("notMapped")}</option>
                      {job.headers.filter(Boolean).map((h) => <option key={h} value={h}>{h}</option>)}
                    </select>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {m.map.isError && <p className="text-red-700">{t("mapError")}</p>}
          <Button disabled={m.map.isPending || job.columns.some((c) => c.required && !mapping[c.key])}
            onClick={() => m.map.mutate({ job, columns: Object.fromEntries(Object.entries(mapping).filter(([, v]) => v)) }, { onSuccess: () => setRemap(false) })}>
            {t("check")}
          </Button>
        </>
      )}
      {step === "validating" && <Waiting text={t("validating")} />}
      {step === "ready" && (
        <>
          <p>{t("summary", { total: job.total_rows ?? 0, valid: job.valid_rows ?? 0, errors: job.error_rows ?? 0 })}</p>
          {job.has_error_report && <a className="underline" href={importErrorsHref(BFF, job.id)}>{t("downloadErrors")}</a>}
          <div className="flex gap-2">
            <Button disabled={m.confirm.isPending || !job.valid_rows} onClick={() => m.confirm.mutate(job)}>{t("confirm", { count: job.valid_rows ?? 0 })}</Button>
            <Button variant="ghost" onClick={() => setRemap(true)}>{t("remap")}</Button>
          </div>
        </>
      )}
      {step === "importing" && <Waiting text={t("importing")} />}
      {step === "done" && <p className="text-green-800">{t("done", { count: job.valid_rows ?? 0 })}</p>}
      {step === "failed" && <p className="text-red-700">{t(`failure.${failureKey(job.failure)}`)}</p>}
      {(step === "done" || step === "failed") && <Button variant="secondary" onClick={reset}>{t("again")}</Button>}
    </div>
  );
}

function Waiting({ text }: { text: string }) {
  return <div className="space-y-2"><p className="text-slate-600">{text}</p><ProgressBar label={text} /></div>;
}

function failureKey(f?: string | null): string {
  if (!f) return "unknown";
  if (f.startsWith("apply_failed")) return "apply_failed";
  return ["file_rejected", "file_unreadable", "no_header_row"].includes(f) ? f : "unknown";
}
