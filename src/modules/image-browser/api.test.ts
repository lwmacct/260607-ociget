import { afterEach, describe, expect, it, vi } from "vitest";
import { APIError, artifactBlobHref, listImageFiles, listObjectPlatforms, resolveOCIObject } from "./api";
import type { OCISource } from "./model";

const source: OCISource = {
  reference: "ghcr.io/acme/tool:v1",
  platform: "linux/amd64",
  insecure: true,
};

afterEach(() => vi.unstubAllGlobals());

describe("OCI browser API", () => {
  it("resolves an OCI reference with its selected source", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      kind: "artifact", objectId: "sha256:abc", reference: source.reference, blobs: [],
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    await resolveOCIObject(source);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/objects");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: "POST" });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ ref: source.reference, platform: source.platform, insecure: true });
  });

  it("sends normalized directory query parameters", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ path: "/usr/bin", entries: [] }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    await listImageFiles("sha256:abc", "usr//local/../bin");
    const url = new URL(fetchMock.mock.calls[0][0], "https://example.test");
    expect(url.pathname).toBe("/api/objects/sha256%3Aabc/entries");
    expect(url.searchParams.get("path")).toBe("/usr/bin");
  });

  it("returns platform records", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ platforms: [{ os: "linux", architecture: "arm64" }] }), { status: 200, headers: { "Content-Type": "application/json" } })));
    await expect(listObjectPlatforms(source)).resolves.toEqual([{ os: "linux", architecture: "arm64" }]);
  });

  it("builds an encoded artifact blob URL", () => {
    expect(artifactBlobHref("sha256:object", "sha256:blob"))
      .toBe("/api/objects/sha256%3Aobject/blobs/sha256%3Ablob");
  });

  it("surfaces Huma error details", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ detail: "path not found" }), { status: 404, headers: { "Content-Type": "application/problem+json" } })));
    await expect(listImageFiles("sha256:abc", "/missing")).rejects.toEqual(new APIError("path not found", 404));
  });
});
