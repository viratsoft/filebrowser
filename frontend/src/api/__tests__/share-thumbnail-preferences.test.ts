import { beforeEach, describe, expect, it, vi } from "vitest";
import * as shares from "../share";
import * as publicApi from "../pub";
import { imagePreviewURL } from "../image-preview-url";
import { createURL, fetchJSON, fetchURL, removePrefix } from "../utils";

vi.mock("../utils", () => ({
  fetchURL: vi.fn(),
  fetchJSON: vi.fn(),
  removePrefix: vi.fn(),
  createURL: vi.fn(() => "public-thumbnail-url"),
}));
vi.mock("@/utils/constants", () => ({
  baseURL: "",
  origin: "https://example.test",
  tusSettings: null,
}));
vi.mock("tus-js-client", () => ({}));

describe("share thumbnail preferences and URLs", () => {
  it("refreshes login thumbnails once so the shared server cache gets populated", () => {
    const modified = "2026-11-11T00:00:00Z";
    imagePreviewURL("/photo.png", modified, "thumb");
    expect(createURL).toHaveBeenCalledWith("api/preview/thumb/photo.png", {
      inline: "true",
      key: Date.parse(modified),
      thumbnailVersion: "2",
    });
  });

  it("uses a distinct public viewer preview URL with only the share token", () => {
    publicApi.getImagePreviewURL({
      hash: "hash",
      path: "/photo #1.png",
      modified: "modified",
      token: "share-token",
    } as Resource);
    expect(createURL).toHaveBeenCalledWith(
      "api/public/preview/hash/photo #1.png",
      { v: "modified", token: "share-token" }
    );
  });

  it("authenticates a legacy original image through a password header, not a URL", async () => {
    const blob = new Blob(["image"]);
    vi.mocked(fetchURL).mockResolvedValue({
      blob: async () => blob,
    } as Response);
    const controller = new AbortController();
    expect(
      await publicApi.fetchImage(
        { hash: "hash", path: "/photo #1.png" } as Resource,
        true,
        "p& secret",
        controller.signal
      )
    ).toBe(blob);
    expect(fetchURL).toHaveBeenCalledWith(
      "/api/public/dl/hash/photo%20%231.png?inline=true",
      {
        signal: controller.signal,
        headers: { "X-SHARE-PASSWORD": "p%26%20secret" },
      },
      false
    );
  });
  beforeEach(() => vi.clearAllMocks());

  it("loads preferences using a route that cannot collide with a shared folder path", async () => {
    vi.mocked(fetchJSON).mockResolvedValue({ showThumbnails: true });
    expect(await shares.preferences()).toEqual({ showThumbnails: true });
    expect(fetchJSON).toHaveBeenCalledWith("/api/shares/preferences", {});
  });

  it.each([true, false])(
    "saves only the owner's thumbnail preference (%s)",
    async (enabled) => {
      await shares.updatePreferences(enabled);
      expect(fetchJSON).toHaveBeenCalledWith("/api/shares/preferences", {
        method: "PUT",
        body: JSON.stringify({ showThumbnails: enabled }),
      });
    }
  );

  it("builds a public URL from a relative path and share token, not login credentials", () => {
    publicApi.getThumbnailURL(
      "hash",
      "/nested/photo #1.png",
      "modified",
      "share-token"
    );
    expect(createURL).toHaveBeenCalledWith(
      "api/public/thumb/hash/nested/photo #1.png",
      { v: "modified", token: "share-token" }
    );
  });

  it("collects individual files from the current shared folder and its subfolders", async () => {
    vi.mocked(removePrefix).mockImplementation((value) =>
      value.replace(/^\/share/, "")
    );
    vi.mocked(fetchURL)
      .mockResolvedValueOnce({
        json: async () => ({
          isDir: true,
          items: [
            { name: "direct.jpg", path: "/current/direct.jpg" },
            { name: "nested", isDir: true },
          ],
        }),
      } as Response)
      .mockResolvedValueOnce({
        json: async () => ({
          isDir: true,
          items: [{ name: "nested.mp4", path: "/current/nested/nested.mp4" }],
        }),
      } as Response);
    const signal = new AbortController().signal;
    const urls = await publicApi.getFolderFileDownloadURLs(
      {
        hash: "hash",
        token: "share-token",
        url: "/share/hash/current/",
      } as Resource,
      "share-password",
      signal
    );
    expect(urls).toHaveLength(2);
    expect(fetchURL).toHaveBeenLastCalledWith(
      "/api/public/share/hash/current/nested/?token=share-token",
      { headers: { "X-SHARE-PASSWORD": "share-password" }, signal },
      false
    );
    expect(createURL).toHaveBeenCalledWith(
      "api/public/dl/hash/current/nested/nested.mp4",
      { token: "share-token" }
    );
  });

  it("encodes each share ZIP path before joining the archive list", () => {
    publicApi.getZipDownloadURL("hash", "share-token", [
      "/folder/a,b #1.jpg",
      "/folder/100%.mp4",
    ]);
    expect(createURL).toHaveBeenCalledWith("api/public/dl/hash/", {
      algo: "zip",
      token: "share-token",
      files: "%2Ffolder%2Fa%2Cb%20%231.jpg,%2Ffolder%2F100%25.mp4",
    });
  });

  it("does not add an empty token to an unprotected thumbnail URL", () => {
    publicApi.getThumbnailURL("hash", "photo.png", "modified");
    expect(createURL).toHaveBeenCalledWith("api/public/thumb/hash/photo.png", {
      v: "modified",
    });
  });
});
