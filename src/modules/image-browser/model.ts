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

export interface OCISource {
  insecure: boolean;
  platform: string;
  reference: string;
}

interface OCIObjectBase {
  annotations?: Record<string, string>;
  createdAt: string;
  insecure?: boolean;
  manifestDigest: string;
  manifestMediaType: string;
  objectId: string;
  reference: string;
}

export interface OCIImageObject extends OCIObjectBase {
  kind: "image";
  platform: string;
}

export interface ArtifactBlob {
  annotations?: Record<string, string>;
  digest: string;
  mediaType: string;
  name: string;
  size: number;
}

export interface OCIArtifactObject extends OCIObjectBase {
  artifactType: string;
  blobs: ArtifactBlob[];
  kind: "artifact";
}

export type OCIObject = OCIImageObject | OCIArtifactObject;

export interface ImageBrowserSession {
  directory: ImageDirectory;
  kind: "image";
  object: OCIImageObject;
  source: OCISource;
}

export interface ArtifactSession {
  kind: "artifact";
  object: OCIArtifactObject;
  source: OCISource;
}

export type BrowserSession = ImageBrowserSession | ArtifactSession;

export const DEFAULT_OCI_SOURCE: OCISource = {
  insecure: false,
  platform: "linux/amd64",
  reference: "ghcr.io/lwmacct/260607-ociget:latest",
};
