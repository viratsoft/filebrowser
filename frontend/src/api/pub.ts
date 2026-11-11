import { fetchURL, removePrefix, createURL, StatusError } from "./utils";
import { imagePreviewURL } from "./image-preview-url";
import { baseURL } from "@/utils/constants";
import * as tus from "tus-js-client";
import { origin, tusSettings } from "@/utils/constants";
import { encodePath } from "@/utils/url";
import {
  publicUploadConflictError,
  publicUploadConflictHeader,
} from "./public-upload-conflict";

export { isPublicUploadFileExistsError } from "./public-upload-conflict";

const publicUploads: Record<string, tus.Upload> = {};
const publicUploadAttemptHeader = "X-FileBrowser-Public-Upload-ID";

function publicUploadAttemptID() {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();

  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join(
    ""
  );
}

function completedPublicUpload(error: Error | tus.DetailedError) {
  return (
    error instanceof tus.DetailedError &&
    error.originalResponse?.getHeader(
      "X-FileBrowser-Public-Upload-Complete"
    ) === "1"
  );
}

function isMissingPublicUpload(error: Error | tus.DetailedError) {
  return (
    error instanceof tus.DetailedError &&
    error.originalResponse?.getStatus() === 404
  );
}

export async function fetch(
  url: string,
  password: string = "",
  signal?: AbortSignal,
  token = ""
) {
  url = removePrefix(url);

  const res = await fetchURL(
    `/api/public/share${url}${token ? `?${new URLSearchParams({ token })}` : ""}`,
    {
      headers: { "X-SHARE-PASSWORD": encodeURIComponent(password) },
      signal,
    },
    false
  );

  const data = (await res.json()) as Resource;
  data.url = `/share${url}`;

  if (data.isDir) {
    if (!data.url.endsWith("/")) data.url += "/";
    // Be tolerant of older servers that serialized an empty directory as null.
    data.items = (data.items || []).map((item: any, index: any) => {
      item.index = index;
      item.url = `${data.url}${encodeURIComponent(item.name)}`;

      if (item.isDir) {
        item.url += "/";
      }

      return item;
    });
  }

  return data;
}

export async function folderSize(
  url: string,
  password: string,
  token: string,
  signal: AbortSignal
): Promise<{ size: number; incomplete?: boolean }> {
  const query = token ? `?${new URLSearchParams({ token })}` : "";
  const response = await fetchURL(
    `/api/public/size${removePrefix(url)}${query}`,
    {
      signal,
      cache: "no-store",
      referrerPolicy: "no-referrer",
      headers: { "X-SHARE-PASSWORD": encodeURIComponent(password) },
    },
    false
  );
  return response.json();
}

export function getDownloadURL(res: Resource, inline = false) {
  const params = {
    ...(inline && { inline: "true" }),
    ...(res.token && { token: res.token }),
  };

  return createURL("api/public/dl/" + res.hash + res.path, params);
}

export function getZipDownloadURL(
  hash: string,
  token: string,
  paths: string[]
) {
  return createURL("api/public/dl/" + hash + "/", {
    algo: "zip",
    // The existing archive API decodes each entry after splitting the list.
    files: paths.map((path) => encodeURIComponent(path)).join(","),
    ...(token && { token }),
  });
}

export function getFolderArchiveURL(folder: Resource) {
  return createURL("api/public/folder-download/" + folder.hash + folder.path, {
    ...(folder.token && { token: folder.token }),
  });
}

export async function getZipDownloadTotals(
  archiveURL: string,
  password: string,
  signal: AbortSignal
): Promise<{ fileCount: number; size: number }> {
  const url = new URL(archiveURL);
  url.searchParams.set("manifest", "totals");
  const response = await globalThis.fetch(url, {
    signal,
    cache: "no-store",
    redirect: "error",
    referrerPolicy: "no-referrer",
    headers: { "X-SHARE-PASSWORD": encodeURIComponent(password) },
  });
  if (!response.ok) throw new StatusError(response.statusText, response.status);
  return response.json();
}

export async function prepareFolderDownload(
  folder: Resource,
  password: string,
  signal: AbortSignal
): Promise<{ paths: string[] }> {
  const query = new URLSearchParams({ manifest: "1" });
  if (folder.token) query.set("token", folder.token);
  const response = await fetchURL(
    `/api/public/folder-download${removePrefix(folder.url)}?${query}`,
    {
      signal,
      cache: "no-store",
      referrerPolicy: "no-referrer",
      headers: { "X-SHARE-PASSWORD": encodeURIComponent(password) },
    },
    false
  );
  return response.json();
}

export async function checkDownloads(urls: string[], signal: AbortSignal) {
  const batchSize = 6;
  for (let i = 0; i < urls.length; i += batchSize) {
    if (signal.aborted) return;
    await Promise.all(
      urls.slice(i, i + batchSize).map(async (url) => {
        const response = await globalThis.fetch(url, {
          method: "HEAD",
          signal,
          cache: "no-store",
          referrerPolicy: "no-referrer",
          redirect: "error",
        });
        if (!response.ok)
          throw new StatusError(response.statusText, response.status);
      })
    );
  }
}

export function startDownload(url: string) {
  const link = document.createElement("a");
  link.href = url;
  link.download = "";
  link.referrerPolicy = "no-referrer";
  link.hidden = true;
  document.body.append(link);
  try {
    link.click();
  } finally {
    link.remove();
  }
}

export function getThumbnailURL(
  hash: string,
  path: string,
  modified: string,
  token = ""
) {
  return imagePreviewURL(path, modified, "thumb", { hash, token });
}

export function getImagePreviewURL(file: Resource) {
  return imagePreviewURL(file.path, file.modified, "big", {
    hash: file.hash ?? "",
    token: file.token,
  });
}

// Legacy password shares may have no query token. Fetch image bytes with the
// password header in that case; never put the password into an image URL.
export async function fetchImage(
  file: Resource,
  original: boolean,
  password: string,
  signal: AbortSignal
) {
  const path = file.path.startsWith("/") ? file.path : `/${file.path}`;
  const endpoint = `api/public/${original ? "dl" : "preview"}/${file.hash}${path}`;
  const query = new URLSearchParams({ inline: "true" });
  if (file.token) query.set("token", file.token);
  const response = await fetchURL(
    `/${encodePath(endpoint)}?${query}`,
    {
      signal,
      headers: { "X-SHARE-PASSWORD": encodeURIComponent(password) },
    },
    false
  );
  return response.blob();
}

// This endpoint is separate from authenticated /api/tus. Passwords are sent
// on each request, but never stored by the resumable-upload fingerprint store.
export async function tusUpload(
  hash: string,
  name: string,
  content: File,
  token: string,
  password: string,
  onupload: (event: { loaded: number }) => void
) {
  if (!tusSettings || !tus.isSupported) {
    throw new Error("Resumable uploads are not supported by this browser");
  }
  const key = `${hash}:${name}:${content.size}:${content.lastModified}`;
  const endpoint = new URL(
    `${baseURL}/api/public/tus/${encodeURIComponent(hash)}/${encodeURIComponent(name)}`,
    origin
  );
  if (token) endpoint.searchParams.set("token", token);
  return new Promise<void>((resolve, reject) => {
    // A mobile browser can suspend long enough for an older server to discard
    // its temporary TUS record. Restart that one transfer once from byte zero;
    // the POST remains no-overwrite, so an actual existing file still reports a
    // conflict instead of being overwritten or silently treated as success.
    const startUpload = (resumePrevious: boolean, restarted = false) => {
      const attemptID = publicUploadAttemptID();
      const upload = new tus.Upload(content, {
        endpoint: endpoint.toString(),
        chunkSize: tusSettings.chunkSize,
        retryDelays: [0, 1000, 3000, 5000, 10000, 20000],
        parallelUploads: 1,
        storeFingerprintForResuming: true,
        headers: {
          [publicUploadAttemptHeader]: attemptID,
          ...(password
            ? { "X-SHARE-PASSWORD": encodeURIComponent(password) }
            : {}),
        },
        onShouldRetry(error) {
          const status = error.originalResponse?.getStatus() || 0;
          return ![401, 403, 404, 409].includes(status);
        },
        onProgress(bytesUploaded) {
          onupload({ loaded: bytesUploaded });
        },
        onError(error) {
          delete publicUploads[key];
          if (error.message === "Upload aborted") return reject(error);
          if (completedPublicUpload(error)) {
            return resolve();
          }
          if (!restarted && isMissingPublicUpload(error)) {
            startUpload(false, true);
            return;
          }
          if (error instanceof tus.DetailedError && error.originalResponse) {
            const conflict = publicUploadConflictError(
              error.originalResponse.getStatus(),
              error.originalResponse.getHeader(publicUploadConflictHeader) || ""
            );
            if (conflict) return reject(conflict);
          }
          const message =
            error instanceof tus.DetailedError && error.originalResponse
              ? error.originalResponse.getBody() || "Upload failed"
              : "Upload failed";
          reject(new Error(message));
        },
        onSuccess() {
          delete publicUploads[key];
          resolve();
        },
      });
      publicUploads[key] = upload;
      if (!resumePrevious) {
        upload.start();
        return;
      }
      upload
        .findPreviousUploads()
        .then((previous) => {
          if (previous.length) upload.resumeFromPreviousUpload(previous[0]);
          upload.start();
        })
        .catch(reject);
    };

    startUpload(true);
  });
}

export function abortTusUpload(hash: string, name: string, content: File) {
  const key = `${hash}:${name}:${content.size}:${content.lastModified}`;
  publicUploads[key]?.abort(true);
  delete publicUploads[key];
}
