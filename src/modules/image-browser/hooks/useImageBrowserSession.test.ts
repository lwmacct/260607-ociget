import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { downloadImageArchive, listImageFiles, listObjectPlatforms, resolveOCIObject } from "../api";
import { useImageBrowserSession } from "./useImageBrowserSession";
import type { OCIArtifactObject, OCIImageObject } from "../model";

vi.mock("../api", () => ({
  downloadImageArchive: vi.fn(),
  listImageFiles: vi.fn(),
  listObjectPlatforms: vi.fn(),
  resolveOCIObject: vi.fn(),
}));

const listFilesMock = vi.mocked(listImageFiles);
const listPlatformsMock = vi.mocked(listObjectPlatforms);
const downloadArchiveMock = vi.mocked(downloadImageArchive);
const resolveObjectMock = vi.mocked(resolveOCIObject);

const image: OCIImageObject = {
  kind: "image",
  objectId: "sha256:abc",
  reference: "ghcr.io/lwmacct/260607-ociget:latest",
  platform: "linux/amd64",
  manifestDigest: "sha256:manifest",
  manifestMediaType: "application/vnd.oci.image.manifest.v1+json",
  createdAt: "2026-08-01T00:00:00Z",
};

const artifact: OCIArtifactObject = {
  kind: "artifact",
  objectId: "sha256:def",
  reference: "ghcr.io/lwmacct/old:artifact",
  artifactType: "application/vnd.example.binary",
  manifestDigest: "sha256:artifact-manifest",
  manifestMediaType: "application/vnd.oci.image.manifest.v1+json",
  createdAt: "2026-08-11T00:00:00Z",
  blobs: [{ digest: "sha256:blob", mediaType: "application/octet-stream", name: "rtty", size: 42 }],
};

beforeEach(() => vi.clearAllMocks());

describe("useImageBrowserSession", () => {
  it("resolves an image and opens the root directory", async () => {
    resolveObjectMock.mockResolvedValue(image);
    listFilesMock.mockResolvedValue({ path: "/", entries: [] });
    const { result } = renderHook(() => useImageBrowserSession());

    await act(async () => { await result.current.openObject(); });

    expect(result.current.session).toEqual({
      kind: "image",
      object: image,
      source: { reference: "ghcr.io/lwmacct/260607-ociget:latest", platform: "linux/amd64", insecure: false },
      directory: { path: "/", entries: [] },
    });
    expect(listFilesMock).toHaveBeenCalledWith("sha256:abc", "/", expect.any(AbortSignal));
  });

  it("opens an artifact without requesting an image directory", async () => {
    resolveObjectMock.mockResolvedValue(artifact);
    const { result } = renderHook(() => useImageBrowserSession());
    act(() => result.current.updateDraft({ reference: artifact.reference }));

    await act(async () => { await result.current.openObject(); });

    expect(result.current.session).toMatchObject({ kind: "artifact", object: artifact });
    expect(listFilesMock).not.toHaveBeenCalled();
  });

  it("navigates and clears selection using the immutable object id", async () => {
    resolveObjectMock.mockResolvedValue(image);
    listFilesMock
      .mockResolvedValueOnce({ path: "/", entries: [] })
      .mockResolvedValueOnce({ path: "/usr", entries: [] });
    const { result } = renderHook(() => useImageBrowserSession());
    await act(async () => { await result.current.openObject(); });
    act(() => result.current.setSelectedPaths(["/bin/tool"]));
    await act(async () => { await result.current.navigate("/usr"); });

    expect(result.current.selectedPaths).toEqual([]);
    expect(listFilesMock).toHaveBeenLastCalledWith("sha256:abc", "/usr", expect.any(AbortSignal));
  });

  it("chooses linux/amd64 when discovering platforms", async () => {
    listPlatformsMock.mockResolvedValue([
      { os: "linux", architecture: "arm64" },
      { os: "linux", architecture: "amd64" },
    ]);
    const { result } = renderHook(() => useImageBrowserSession());
    await act(async () => { await result.current.discoverPlatforms(); });
    await waitFor(() => expect(result.current.platformOptions).toEqual(["linux/arm64", "linux/amd64"]));
    expect(result.current.draft.platform).toBe("linux/amd64");
  });

  it("downloads selected files from an image object", async () => {
    resolveObjectMock.mockResolvedValue(image);
    listFilesMock.mockResolvedValue({ path: "/", entries: [] });
    downloadArchiveMock.mockResolvedValue();
    const { result } = renderHook(() => useImageBrowserSession());
    await act(async () => { await result.current.openObject(); });
    act(() => result.current.setSelectedPaths(["/bin/tool"]));
    await act(async () => { await result.current.downloadSelected(); });
    expect(downloadArchiveMock).toHaveBeenCalledWith("sha256:abc", ["/bin/tool"]);
  });
});
