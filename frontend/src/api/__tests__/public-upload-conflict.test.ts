import { describe, expect, it } from "vitest";
import {
  isPublicUploadFileExistsError,
  publicUploadConflictError,
} from "../public-upload-conflict";

describe("public upload conflict classification", () => {
  it("recognizes only a confirmed filename conflict", () => {
    const error = publicUploadConflictError(409, "file_exists");
    expect(isPublicUploadFileExistsError(error)).toBe(true);
    expect(error?.message).toBe("File already exists");
  });

  it.each(["offset_mismatch", "upload_in_progress", "", "unknown"])(
    "does not call %s a filename conflict",
    (reason) => {
      const error = publicUploadConflictError(409, reason);
      expect(isPublicUploadFileExistsError(error)).toBe(false);
      expect(error?.message).not.toContain("File already exists");
    }
  );

  it("uses a specific message for an offset mismatch", () => {
    expect(publicUploadConflictError(409, "offset_mismatch")?.message).toBe(
      "Upload position changed. Please retry this file."
    );
  });

  it("does not infer conflicts from generic error text or other statuses", () => {
    expect(isPublicUploadFileExistsError(new Error("409 Conflict"))).toBe(
      false
    );
    expect(publicUploadConflictError(403, "file_exists")).toBeUndefined();
    expect(publicUploadConflictError(404, "file_exists")).toBeUndefined();
  });
});
