import { describe, expect, it } from "vitest";
import type { ImagePlatform } from "./model";
import {
  choosePlatform,
  downloadHref,
  imagePathCrumbs,
  normalizeImagePath,
  parentImagePath,
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

  it("builds downloads from an immutable image id", () => {
    const href = new URL(downloadHref("sha256:abc", "/usr//bin/tool"), "https://example.test");
    expect(href.pathname).toBe("/api/images/sha256%3Aabc/file");
    expect(href.searchParams.get("path")).toBe("/usr/bin/tool");
  });
});
