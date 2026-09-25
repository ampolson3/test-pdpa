"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { FileDropzone, ProgressBar } from "@pdpa/ui";
import {
  createApiClient,
  fileDownloadHref,
  uploadFile,
  useFileStatus,
  type StoredFile,
  type UploadProblem,
} from "@pdpa/api-client";

const BFF = "/api/bff";

type Item = { key: string; name: string; progress: number; file?: StoredFile; problem?: UploadProblem };

/**
 * Upload / download component (PLT-09): drag-and-drop or pick, upload progress, then the virus-scan
 * status until the file is clean (download link) or rejected. `onUploaded` gives the parent the file
 * id to attach to its record.
 */
export function FileUploader({ onUploaded, multiple = false }: { onUploaded?: (file: StoredFile) => void; multiple?: boolean }) {
  const t = useTranslations("files");
  const [items, setItems] = useState<Item[]>([]);

  const update = (key: string, patch: Partial<Item>) =>
    setItems((prev) => prev.map((it) => (it.key === key ? { ...it, ...patch } : it)));

  const start = (files: File[]) => {
    for (const f of files) {
      const key = `${f.name}-${f.size}-${Date.now()}-${Math.random()}`;
      setItems((prev) => [...prev, { key, name: f.name, progress: 0 }]);
      uploadFile(BFF, f, (p) => update(key, { progress: p }))
        .then((stored) => {
          update(key, { progress: 1, file: stored });
          onUploaded?.(stored);
        })
        .catch((problem: UploadProblem) => update(key, { problem }));
    }
  };

  return (
    <div className="space-y-3">
      <FileDropzone
        onFiles={start}
        multiple={multiple}
        accept=".pdf,.png,.jpg,.jpeg,.docx,.xlsx,.pptx,.csv,.txt"
        inputLabel={t("choose")}
        prompt={
          <span>
            {t("dropHere")}
            <br />
            <span className="text-xs text-slate-500">{t("limits")}</span>
          </span>
        }
      />
      <ul className="space-y-2">
        {items.map((it) => (
          <li key={it.key} className="rounded-md border border-slate-200 bg-white p-3 text-sm">
            <div className="mb-1 flex justify-between gap-2">
              <span className="truncate">{it.name}</span>
              {it.file ? <ScanStatus file={it.file} /> : null}
            </div>
            {it.problem ? (
              <p className="text-red-700">{problemText(t, it.problem)}</p>
            ) : !it.file ? (
              <ProgressBar value={it.progress} label={t("uploading")} />
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}

// Problem codes contain dots (files.too_large), which next-intl reads as nesting — keys use "_".
function problemText(t: ReturnType<typeof useTranslations<"files">>, problem: UploadProblem): string {
  const key = `error.${(problem.code ?? "").replace(/\./g, "_")}`;
  return t.has(key) ? t(key) : t("error.generic");
}

function ScanStatus({ file }: { file: StoredFile }) {
  const t = useTranslations("files");
  const client = useMemo(() => createApiClient(BFF), []);
  const { data } = useFileStatus(client, file.id);
  const status = data?.av_status ?? file.av_status;

  switch (status) {
    case "clean":
      return (
        <a className="text-slate-900 underline" href={fileDownloadHref(BFF, file.id)}>
          {t("download")}
        </a>
      );
    case "infected":
      return <span className="text-red-700">{t("status.infected")}</span>;
    case "error":
      return <span className="text-amber-700">{t("status.error")}</span>;
    default:
      return <span className="text-slate-500">{t("status.pending")}</span>;
  }
}
