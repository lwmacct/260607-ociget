import { describe, expect, it } from "vitest";
import type { ImagePlatform } from "./model";
import {
  choosePlatform,
  artifactDirectHref,
  downloadHref,
  imagePathCrumbs,
  normalizeImagePath,
  parentImagePath,
  pinOCIReference,
} from "./utils";

describe("image path utilities", () => {
  it.each([
    ["", "/"],
    ["/", "/"],
    ["usr//local/./bin/", "/usr/local/bin"],
    ["/usr/local/../bin", "/usr/bin"],
    ["../../etc", "/etc"],
  ])("normalizes %j", (input, expected) => {
    expect(normalizeImagePath(input)).toBe(expected);
  });

  it("builds parent paths and breadcrumbs", () => {
    expect(parentImagePath("/usr/local/bin")).toBe("/usr/local");
    expect(parentImagePath("/usr")).toBe("/");
    expect(imagePathCrumbs("/usr/local", "Root")).toEqual([
      { label: "Root", path: "/" },
      { label: "usr", path: "/usr" },
      { label: "local", path: "/usr/local" },
    ]);
  });
});

describe("image source utilities", () => {
  const platforms: ImagePlatform[] = [
    { os: "linux", architecture: "arm64", variant: "v8" },
    { os: "linux", architecture: "amd64" },
  ];

  it("keeps a supported platform and otherwise prefers linux/amd64", () => {
    expect(choosePlatform(platforms, "linux/arm64/v8")).toBe("linux/arm64/v8");
    expect(choosePlatform(platforms, "windows/amd64")).toBe("linux/amd64");
  });

  it("builds direct image downloads", () => {
    const href = new URL(downloadHref({ reference: "ghcr.io/acme/tool:v1", platform: "linux/amd64", insecure: false }, "/usr//bin/tool"), "https://example.test");
    expect(href.pathname).toBe("/ghcr.io/acme/tool:v1/-/usr/bin/tool");
    expect(href.searchParams.get("platform")).toBe("linux/amd64");
  });

  it("escapes image reference delimiters", () => {
    expect(downloadHref({ reference: "registry.example/acme/tool:v1#unsafe", platform: "", insecure: true }, "/bin/app"))
      .toBe("/registry.example/acme/tool:v1%23unsafe/-/bin/app?insecure=1");
  });

  it("pins tagged and port-qualified OCI references", () => {
    expect(pinOCIReference("ghcr.io/acme/tool:v1", "sha256:manifest"))
      .toBe("ghcr.io/acme/tool@sha256:manifest");
    expect(pinOCIReference("registry.example:5000/acme/tool", "sha256:manifest"))
      .toBe("registry.example:5000/acme/tool@sha256:manifest");
    expect(pinOCIReference("ghcr.io/acme/tool@sha256:old", "sha256:new"))
      .toBe("ghcr.io/acme/tool@sha256:new");
  });

  it("builds manifest-pinned artifact direct downloads", () => {
    expect(artifactDirectHref({
      reference: "ghcr.io/acme/tool:v1",
      manifestDigest: "sha256:manifest",
    }, "sha256:blob", true)).toBe(
      "/ghcr.io/acme/tool@sha256:manifest/-/@sha256:blob?insecure=1",
    );
  });
});
