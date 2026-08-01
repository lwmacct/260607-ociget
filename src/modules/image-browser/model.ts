export type EntryType = "directory" | "file" | "symlink" | "other";

export interface ImageEntry {
  linkName?: string;
  mode: number;
  modTime?: string;
  name: string;
  path: string;
  size: number;
  type: EntryType;
}

export interface ImageDirectory {
  entries: ImageEntry[];
  path: string;
}

export interface ImagePlatform {
  architecture: string;
  os: string;
  osVersion?: string;
  variant?: string;
}

export interface ImageSource {
  imageRef: string;
  insecure: boolean;
  platform: string;
}

export interface ImageBrowserSession {
  image: ImageMaterialized;
  directory: ImageDirectory;
  source: ImageSource;
}

export interface ImageMaterialized {
  createdAt: string;
  imageId: string;
  imageRef: string;
  platform: string;
}

export const DEFAULT_IMAGE_SOURCE: ImageSource = {
  imageRef: "ghcr.io/lwmacct/260607-ociget:latest",
  insecure: false,
  platform: "linux/amd64",
};
