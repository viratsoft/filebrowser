import { beforeEach, describe, expect, it, vi } from "vitest";
import * as shares from "../share";
import * as publicApi from "../pub";
import { createURL, fetchJSON } from "../utils";

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

  it("does not add an empty token to an unprotected thumbnail URL", () => {
    publicApi.getThumbnailURL("hash", "photo.png", "modified");
    expect(createURL).toHaveBeenCalledWith("api/public/thumb/hash/photo.png", {
      v: "modified",
    });
  });
});
