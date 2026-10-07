import { beforeEach, describe, expect, it, vi } from "vitest";
import { isPublicUploadFileExistsError, tusUpload } from "../pub";

const scenario = vi.hoisted(() => ({
  status: 409,
  headers: {} as Record<string, string>,
}));

vi.mock("../utils", () => ({
  fetchURL: vi.fn(),
  removePrefix: vi.fn(),
  createURL: vi.fn(),
}));
vi.mock("@/utils/constants", () => ({
  baseURL: "",
  origin: "https://example.test",
  tusSettings: { chunkSize: 1024 },
}));
vi.mock("tus-js-client", () => {
  class DetailedError extends Error {
    originalResponse = {
      getStatus: () => scenario.status,
      getHeader: (header: string) => scenario.headers[header] || null,
      getBody: () => `${scenario.status} Conflict`,
    };
  }
  return {
    isSupported: true,
    DetailedError,
    Upload: class {
      options: { onError: (error: Error) => void };

      constructor(_file: File, options: { onError: (error: Error) => void }) {
        this.options = options;
      }

      findPreviousUploads() {
        return Promise.resolve([]);
      }

      start() {
        this.options.onError(new DetailedError("Request failed"));
      }
    },
  };
});

describe("public TUS error callback", () => {
  beforeEach(() => {
    scenario.status = 409;
    scenario.headers = {};
  });

  const upload = () =>
    tusUpload(
      "share",
      "report.txt",
      new File(["abc"], "report.txt"),
      "",
      "",
      vi.fn()
    );

  it("passes confirmed filename conflicts to the filename toast predicate", async () => {
    scenario.headers["X-FileBrowser-Public-Upload-Conflict"] = "file_exists";
    const error = await upload().catch((failure: unknown) => failure);
    expect(isPublicUploadFileExistsError(error)).toBe(true);
  });

  it.each(["offset_mismatch", "upload_in_progress", "", "unknown"])(
    "does not turn %s into a filename conflict in the upload callback",
    async (reason) => {
      scenario.headers["X-FileBrowser-Public-Upload-Conflict"] = reason;
      const error = await upload().catch((failure: unknown) => failure);
      expect(error).toBeInstanceOf(Error);
      expect(isPublicUploadFileExistsError(error)).toBe(false);
      expect((error as Error).message).not.toContain("File already exists");
    }
  );

  it("preserves completed-attempt recovery before conflict classification", async () => {
    scenario.headers = {
      "X-FileBrowser-Public-Upload-Conflict": "file_exists",
      "X-FileBrowser-Public-Upload-Complete": "1",
    };
    await expect(upload()).resolves.toBeUndefined();
  });

  it("does not classify an unauthorized response using a conflict header", async () => {
    scenario.status = 401;
    scenario.headers["X-FileBrowser-Public-Upload-Conflict"] = "file_exists";
    const error = await upload().catch((failure: unknown) => failure);
    expect(error).toBeInstanceOf(Error);
    expect(isPublicUploadFileExistsError(error)).toBe(false);
    expect((error as Error).message).toContain("401");
  });
});
