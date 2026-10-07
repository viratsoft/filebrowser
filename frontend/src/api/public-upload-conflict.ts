export const publicUploadConflictHeader =
  "X-FileBrowser-Public-Upload-Conflict";

class PublicUploadConflictError extends Error {
  readonly reason: string;

  constructor(reason: string, message: string) {
    super(message);
    this.name = "PublicUploadConflictError";
    this.reason = reason;
  }
}

// Only an explicit server filename-conflict code may produce the existing-file
// toast. Unknown/legacy 409 responses must not be guessed from their text.
export function publicUploadConflictError(status: number, reason: string) {
  if (status !== 409) return undefined;

  switch (reason) {
    case "file_exists":
      return new PublicUploadConflictError(reason, "File already exists");
    case "offset_mismatch":
      return new PublicUploadConflictError(
        reason,
        "Upload position changed. Please retry this file."
      );
    case "upload_in_progress":
      return new PublicUploadConflictError(
        reason,
        "An unfinished upload already exists for this file. Please retry later."
      );
    default:
      return new PublicUploadConflictError(
        "unknown",
        "Upload conflict. Please retry this file."
      );
  }
}

export function isPublicUploadFileExistsError(error: unknown) {
  return (
    error instanceof PublicUploadConflictError && error.reason === "file_exists"
  );
}
