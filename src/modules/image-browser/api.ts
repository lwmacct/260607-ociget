import type { ImageDirectory, ImageMaterialized, ImagePlatform, ImageSource } from "./model";
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

export async function materializeImage(
  source: ImageSource,
  signal?: AbortSignal,
): Promise<ImageMaterialized> {
  return requestJSON<ImageMaterialized>("/api/images", signal, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      ref: source.imageRef.trim(),
      platform: source.platform.trim() || undefined,
      insecure: source.insecure || undefined,
    }),
  });
}

export async function listImageFiles(
  imageId: string,
  path: string,
  signal?: AbortSignal,
): Promise<ImageDirectory> {
  const query = new URLSearchParams();
  query.set("path", normalizeImagePath(path));
  return requestJSON<ImageDirectory>(`/api/images/${encodeURIComponent(imageId)}/entries?${query.toString()}`, signal);
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
  imageId: string,
  paths: string[],
): Promise<void> {
  const response = await fetch(`/api/images/${encodeURIComponent(imageId)}/archive`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      paths,
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

async function requestJSON<T>(url: string, signal?: AbortSignal, init: RequestInit = {}): Promise<T> {
  const response = await fetch(url, { ...init, signal });
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
