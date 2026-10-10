import { useQuery } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type StoredFile = components["schemas"]["StoredFile"];
export type UploadProblem = components["schemas"]["Problem"];

/**
 * POST /admin/v1/platform/files through the BFF (PLT-09). Uses XMLHttpRequest because fetch has no
 * upload-progress events. Resolves with the stored file (scan still pending) or rejects with the
 * API's problem+json (e.g. files.too_large, files.type_not_allowed).
 */
export function uploadFile(
  bffBaseUrl: string,
  file: File,
  onProgress?: (fraction: number) => void,
  signal?: AbortSignal,
): Promise<StoredFile> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${bffBaseUrl}/admin/v1/platform/files`);
    xhr.responseType = "json";
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress?.(e.loaded / e.total);
    };
    xhr.onload = () => {
      if (xhr.status === 201) resolve(xhr.response as StoredFile);
      else reject((xhr.response as UploadProblem) ?? { status: xhr.status, code: "internal", title: "", type: "about:blank" });
    };
    xhr.onerror = () => reject({ status: 0, code: "network", title: "", type: "about:blank" } as UploadProblem);
    signal?.addEventListener("abort", () => xhr.abort());
    const body = new FormData();
    body.append("file", file);
    xhr.send(body);
  });
}

/** Link that downloads a file: the BFF answers 302 to a short-lived signed URL. */
export function fileDownloadHref(bffBaseUrl: string, id: string): string {
  return `${bffBaseUrl}/admin/v1/platform/files/${id}/download`;
}

/** GET /admin/v1/platform/files/{id}, polled every 2 s while the virus scan is pending. */
export function useFileStatus(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["platform", "files", id],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/files/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
    refetchInterval: (q) => (q.state.data?.av_status === "pending" ? 2_000 : false),
  });
}
