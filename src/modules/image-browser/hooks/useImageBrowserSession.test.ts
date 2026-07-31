import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { downloadImageArchive, listImageFiles, listImagePlatforms } from "../api";
import { useImageBrowserSession } from "./useImageBrowserSession";

vi.mock("../api", () => ({
  downloadImageArchive: vi.fn(),
  listImageFiles: vi.fn(),
  listImagePlatforms: vi.fn(),
}));

const listFilesMock = vi.mocked(listImageFiles);
const listPlatformsMock = vi.mocked(listImagePlatforms);
const downloadArchiveMock = vi.mocked(downloadImageArchive);

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useImageBrowserSession", () => {
  it("commits a source only after a successful directory request", async () => {
    listFilesMock.mockResolvedValue({ path: "", entries: [] });
    const { result } = renderHook(() => useImageBrowserSession());

    await act(async () => {
      await result.current.openImage();
    });

    expect(result.current.session).toEqual({
      source: { imageRef: "alpine:latest", platform: "linux/amd64", insecure: false },
      directory: { path: "/", entries: [] },
    });
  });

  it("keeps only the latest directory response", async () => {
    let resolveFirst: ((value: { path: string; entries: [] }) => void) | undefined;
    const first = new Promise<{ path: string; entries: [] }>((resolve) => {
      resolveFirst = resolve;
    });
    listFilesMock
      .mockReturnValueOnce(first)
      .mockResolvedValueOnce({ path: "/second", entries: [] });
    const { result } = renderHook(() => useImageBrowserSession());

    let firstRequest: Promise<boolean>;
    act(() => {
      firstRequest = result.current.openImage();
    });
    await act(async () => {
      result.current.updateDraft({ imageRef: "example.com/second:v1" });
    });
    let secondRequest: Promise<boolean>;
    act(() => {
      secondRequest = result.current.openImage();
    });
    await act(async () => {
      await secondRequest;
    });
    await act(async () => {
      resolveFirst?.({ path: "/first", entries: [] });
      await firstRequest;
    });

    expect(result.current.session?.source.imageRef).toBe("example.com/second:v1");
    expect(result.current.session?.directory.path).toBe("/second");
  });

  it("clears file selection after successful navigation", async () => {
    listFilesMock
      .mockResolvedValueOnce({ path: "/", entries: [] })
      .mockResolvedValueOnce({ path: "/usr", entries: [] });
    const { result } = renderHook(() => useImageBrowserSession());
    await act(async () => {
      await result.current.openImage();
    });
    act(() => result.current.setSelectedPaths(["/bin/tool"]));

    await act(async () => {
      await result.current.navigate("/usr");
    });

    expect(result.current.selectedPaths).toEqual([]);
    expect(result.current.session?.directory.path).toBe("/usr");
  });

  it("chooses linux/amd64 when discovering platforms", async () => {
    listPlatformsMock.mockResolvedValue([
      { os: "linux", architecture: "arm64" },
      { os: "linux", architecture: "amd64" },
    ]);
    const { result } = renderHook(() => useImageBrowserSession());

    await act(async () => {
      await result.current.discoverPlatforms();
    });

    await waitFor(() => expect(result.current.platformOptions).toEqual([
      "linux/arm64",
      "linux/amd64",
    ]));
    expect(result.current.draft.platform).toBe("linux/amd64");
  });

  it("downloads selected files from the committed source", async () => {
    listFilesMock.mockResolvedValue({ path: "/", entries: [] });
    downloadArchiveMock.mockResolvedValue();
    const { result } = renderHook(() => useImageBrowserSession());
    await act(async () => {
      await result.current.openImage();
    });
    act(() => result.current.setSelectedPaths(["/bin/tool"]));

    await act(async () => {
      await result.current.downloadSelected();
    });

    expect(downloadArchiveMock).toHaveBeenCalledWith(
      result.current.session?.source,
      ["/bin/tool"],
    );
  });
});
