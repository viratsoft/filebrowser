import { createURL } from "./utils";

export function imagePreviewURL(
  path: string,
  modified: string,
  size: "thumb" | "big",
  share?: { hash: string; token?: string }
) {
  const relativePath = path.startsWith("/") ? path : `/${path}`;
  if (share) {
    const endpoint = size === "thumb" ? "thumb" : "preview";
    return createURL(`api/public/${endpoint}/${share.hash}${relativePath}`, {
      v: modified,
      ...(share.token && { token: share.token }),
    });
  }

  return createURL(`api/preview/${size}${relativePath}`, {
    inline: "true",
    key: Date.parse(modified),
    // Refresh thumbnails once after switching to the shared server renderer.
    ...(size === "thumb" && { thumbnailVersion: "2" }),
  });
}
