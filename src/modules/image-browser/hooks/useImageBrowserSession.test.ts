import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { downloadImageArchive, listImageFiles, listImagePlatforms, materializeImage } from "../api";
import { useImageBrowserSession } from "./useImageBrowserSession";

vi.mock("../api", () => ({
  downloadImageArchive: vi.fn(),
  listImageFiles: vi.fn(),
  listImagePlatforms: vi.fn(),
  materializeImage: vi.fn(),
}));

const listFilesMock = vi.mocked(listImageFiles);
const listPlatformsMock = vi.mocked(listImagePlatforms);
const downloadArchiveMock = vi.mocked(downloadImageArchive);
const materializeMock = vi.mocked(materializeImage);

const image = {
  imageId: "sha256:abc",
  imageRef: "alpine:latest",
  platform: "linux/amd64",
  createdAt: "2026-08-01T00:00:00Z",
};

beforeEach(() => vi.clearAllMocks());

describe("useImageBrowserSession", () => {
  it("materializes and opens the root directory", async () => {
    materializeMock.mockResolvedValue(image);
    listFilesMock.mockResolvedValue({ path: "/", entries: [] });
    const { result } = renderHook(() => useImageBrowserSession());

    await act(async () => { await result.current.openImage(); });

    expect(result.current.session).toEqual({
      image,
      source: { imageRef: "alpine:latest", platform: "linux/amd64", insecure: false },
      directory: { path: "/", entries: [] },
    });
    expect(listFilesMock).toHaveBeenCalledWith("sha256:abc", "/", expect.any(AbortSignal));
  });

  it("navigates and clears selection using the immutable image id", async () => {
    materializeMock.mockResolvedValue(image);
    listFilesMock
      .mockResolvedValueOnce({ path: "/", entries: [] })
      .mockResolvedValueOnce({ path: "/usr", entries: [] });
    const { result } = renderHook(() => useImageBrowserSession());
    await act(async () => { await result.current.openImage(); });
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

  it("downloads selected files from the immutable image", async () => {
    materializeMock.mockResolvedValue(image);
    listFilesMock.mockResolvedValue({ path: "/", entries: [] });
    downloadArchiveMock.mockResolvedValue();
    const { result } = renderHook(() => useImageBrowserSession());
    await act(async () => { await result.current.openImage(); });
    act(() => result.current.setSelectedPaths(["/bin/tool"]));
    await act(async () => { await result.current.downloadSelected(); });
    expect(downloadArchiveMock).toHaveBeenCalledWith("sha256:abc", ["/bin/tool"]);
  });
});
