import { afterEach, describe, expect, it, vi } from "vitest";
import { APIError, listImageFiles, listImagePlatforms, materializeImage } from "./api";
import type { ImageSource } from "./model";

const source: ImageSource = {
  imageRef: "ghcr.io/acme/tool:v1",
  platform: "linux/amd64",
  insecure: true,
};

afterEach(() => vi.unstubAllGlobals());

describe("image browser API", () => {
  it("materializes an image with its committed source", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      imageId: "sha256:abc", imageRef: source.imageRef, platform: source.platform, createdAt: "2026-08-01T00:00:00Z",
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    await materializeImage(source);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/images");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: "POST" });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ ref: source.imageRef, platform: source.platform, insecure: true });
  });

  it("sends normalized directory query parameters", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ path: "/usr/bin", entries: [] }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    await listImageFiles("sha256:abc", "usr//local/../bin");
    const url = new URL(fetchMock.mock.calls[0][0], "https://example.test");
    expect(url.pathname).toBe("/api/images/sha256%3Aabc/entries");
    expect(url.searchParams.get("path")).toBe("/usr/bin");
  });

  it("returns platform records", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ platforms: [{ os: "linux", architecture: "arm64" }] }), { status: 200, headers: { "Content-Type": "application/json" } })));
    await expect(listImagePlatforms(source)).resolves.toEqual([{ os: "linux", architecture: "arm64" }]);
  });

  it("surfaces Huma error details", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ detail: "path not found" }), { status: 404, headers: { "Content-Type": "application/problem+json" } })));
    await expect(listImageFiles("sha256:abc", "/missing")).rejects.toEqual(new APIError("path not found", 404));
  });
});
