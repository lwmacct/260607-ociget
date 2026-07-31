import { DEFAULT_IMAGE_SOURCE, type EntryType, type ImagePlatform, type ImageSource } from "./model";

export interface PathCrumb {
  label: string;
  path: string;
}

export function normalizeImagePath(value: string): string {
  const parts: string[] = [];
  for (const segment of value.trim().split("/")) {
    if (!segment || segment === ".") continue;
    if (segment === "..") {
      parts.pop();
      continue;
    }
    parts.push(segment);
  }
  return parts.length ? `/${parts.join("/")}` : "/";
}

export function parentImagePath(value: string): string {
  const parts = normalizeImagePath(value).split("/").filter(Boolean);
  parts.pop();
  return parts.length ? `/${parts.join("/")}` : "/";
}

export function imagePathCrumbs(value: string, rootLabel: string): PathCrumb[] {
  const parts = normalizeImagePath(value).split("/").filter(Boolean);
  return [
    { label: rootLabel, path: "/" },
    ...parts.map((label, index) => ({
      label,
      path: `/${parts.slice(0, index + 1).join("/")}`,
    })),
  ];
}

export function formatFileSize(size: number, type: EntryType): string {
  if (type === "directory" || size < 0) return "-";
  if (size < 1024) return `${size} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let value = size / 1024;
  let unit = units[0];
  for (let index = 1; index < units.length && value >= 1024; index += 1) {
    value /= 1024;
    unit = units[index];
  }
  return `${value >= 10 ? value.toFixed(1) : value.toFixed(2)} ${unit}`;
}

export function formatFileMode(mode: number): string {
  return mode ? `0${(mode & 0o7777).toString(8)}` : "-";
}

export function formatFileDate(value: string | undefined, locale: string): string {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() <= 1) return "-";
  return date.toLocaleString(locale);
}

export function platformValue(platform: ImagePlatform): string {
  return [platform.os, platform.architecture, platform.variant].filter(Boolean).join("/");
}

export function choosePlatform(platforms: ImagePlatform[], current: string): string {
  const values = platforms.map(platformValue).filter(Boolean);
  if (current && values.includes(current)) return current;
  if (values.includes(DEFAULT_IMAGE_SOURCE.platform)) return DEFAULT_IMAGE_SOURCE.platform;
  return values[0] ?? current;
}

export function sourceKey(source: Pick<ImageSource, "imageRef" | "insecure">): string {
  return `${source.insecure ? "insecure" : "secure"}:${source.imageRef.trim()}`;
}

export function sameImageSource(left: ImageSource, right: ImageSource): boolean {
  return left.imageRef === right.imageRef
    && left.platform === right.platform
    && left.insecure === right.insecure;
}

export function downloadHref(source: ImageSource, path: string): string {
  const query = new URLSearchParams({
    ref: source.imageRef,
    path: normalizeImagePath(path),
  });
  if (source.platform) query.set("platform", source.platform);
  if (source.insecure) query.set("insecure", "true");
  return `/download?${query.toString()}`;
}
