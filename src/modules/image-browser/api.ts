import type { ImageDirectory, ImagePlatform, ImageSource } from "./model";
import { normalizeImagePath } from "./utils";

interface PlatformsResponse {
  platforms: ImagePlatform[];
}

export class APIError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = "APIError";
  }
}

export async function listImageFiles(
  source: ImageSource,
  path: string,
  signal?: AbortSignal,
): Promise<ImageDirectory> {
  const query = sourceQuery(source);
  query.set("path", normalizeImagePath(path));
  return requestJSON<ImageDirectory>(`/api/images/files?${query.toString()}`, signal);
}

export async function listImagePlatforms(
  source: Pick<ImageSource, "imageRef" | "insecure">,
  signal?: AbortSignal,
): Promise<ImagePlatform[]> {
  const query = new URLSearchParams({ ref: source.imageRef.trim() });
  if (source.insecure) query.set("insecure", "true");
  const response = await requestJSON<PlatformsResponse>(
    `/api/images/platforms?${query.toString()}`,
    signal,
  );
  return response.platforms;
}

export async function downloadImageArchive(
  source: ImageSource,
  paths: string[],
): Promise<void> {
  const response = await fetch("/download/archive", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      ref: source.imageRef,
      paths,
      platform: source.platform || undefined,
      insecure: source.insecure || undefined,
    }),
  });
  if (!response.ok) {
    throw await responseError(response);
  }

  const blob = await response.blob();
  const href = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = href;
  link.download = "image-files.tar";
  document.body.appendChild(link);
  link.click();
  link.remove();
  globalThis.setTimeout(() => URL.revokeObjectURL(href), 0);
}

async function requestJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(url, { signal });
  if (!response.ok) {
    throw await responseError(response);
  }
  return response.json() as Promise<T>;
}

async function responseError(response: Response): Promise<APIError> {
  const fallback = `HTTP ${response.status}`;
  const contentType = response.headers.get("Content-Type") ?? "";
  if (contentType.includes("json")) {
    const body = await response.json().catch(() => null) as { detail?: unknown } | null;
    const detail = typeof body?.detail === "string" ? body.detail.trim() : "";
    return new APIError(detail || fallback, response.status);
  }
  const body = await response.text().catch(() => "");
  return new APIError(body.trim() || fallback, response.status);
}

function sourceQuery(source: ImageSource): URLSearchParams {
  const query = new URLSearchParams({ ref: source.imageRef.trim() });
  if (source.platform.trim()) query.set("platform", source.platform.trim());
  if (source.insecure) query.set("insecure", "true");
  return query;
}
