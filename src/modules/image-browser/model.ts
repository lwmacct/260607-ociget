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
  directory: ImageDirectory;
  source: ImageSource;
}

export const DEFAULT_IMAGE_SOURCE: ImageSource = {
  imageRef: "alpine:latest",
  insecure: false,
  platform: "linux/amd64",
};
