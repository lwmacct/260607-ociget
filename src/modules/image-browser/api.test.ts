import { afterEach, describe, expect, it, vi } from "vitest";
import { APIError, listImageFiles, listImagePlatforms } from "./api";
import type { ImageSource } from "./model";

const source: ImageSource = {
  imageRef: "ghcr.io/acme/tool:v1",
  platform: "linux/amd64",
  insecure: true,
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("image browser API", () => {
  it("sends normalized directory query parameters", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      path: "/usr/bin",
      entries: [],
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await listImageFiles(source, "usr//local/../bin");

    const url = new URL(fetchMock.mock.calls[0][0], "https://example.test");
    expect(url.pathname).toBe("/api/images/files");
    expect(url.searchParams.get("ref")).toBe(source.imageRef);
    expect(url.searchParams.get("path")).toBe("/usr/bin");
    expect(url.searchParams.get("platform")).toBe(source.platform);
    expect(url.searchParams.get("insecure")).toBe("true");
  });

  it("returns platform records", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      platforms: [{ os: "linux", architecture: "arm64" }],
    }), { status: 200, headers: { "Content-Type": "application/json" } })));

    await expect(listImagePlatforms(source)).resolves.toEqual([
      { os: "linux", architecture: "arm64" },
    ]);
  });

  it("surfaces Huma error details", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      detail: "path not found",
    }), { status: 404, headers: { "Content-Type": "application/problem+json" } })));

    await expect(listImageFiles(source, "/missing")).rejects.toEqual(
      new APIError("path not found", 404),
    );
  });
});
