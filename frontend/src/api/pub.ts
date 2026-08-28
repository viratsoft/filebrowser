import { fetchURL, removePrefix, createURL } from "./utils";
import { baseURL } from "@/utils/constants";
import * as tus from "tus-js-client";
import { origin, tusSettings } from "@/utils/constants";

const publicUploads: Record<string, tus.Upload> = {};

export async function fetch(url: string, password: string = "") {
  url = removePrefix(url);

  const res = await fetchURL(
    `/api/public/share${url}`,
    {
      headers: { "X-SHARE-PASSWORD": encodeURIComponent(password) },
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

export function download(
  format: DownloadFormat,
  hash: string,
  token: string,
  ...files: string[]
) {
  let url = `${baseURL}/api/public/dl/${hash}`;

  if (files.length === 1) {
    url += files[0] + "?";
  } else {
    let arg = "";

    for (const file of files) {
      arg += file + ",";
    }

    arg = arg.substring(0, arg.length - 1);
    arg = encodeURIComponent(arg);
    url += `/?files=${arg}&`;
  }

  if (format) {
    url += `algo=${format}&`;
  }

  if (token) {
    url += `token=${token}&`;
  }

  window.open(url);
}

export function getDownloadURL(res: Resource, inline = false) {
  const params = {
    ...(inline && { inline: "true" }),
    ...(res.token && { token: res.token }),
  };

  return createURL("api/public/dl/" + res.hash + res.path, params);
}

// This endpoint is separate from authenticated /api/tus. Passwords are sent
// on each request, but never stored by the resumable-upload fingerprint store.
export async function tusUpload(hash: string, name: string, content: File, token: string, password: string, onupload: (event: { loaded: number }) => void) {
  if (!tusSettings || !tus.isSupported) throw new Error("Resumable uploads are not supported by this browser");
  const key = `${hash}:${name}:${content.size}:${content.lastModified}`;
  const endpoint = new URL(`${baseURL}/api/public/tus/${encodeURIComponent(hash)}/${encodeURIComponent(name)}`, origin);
  if (token) endpoint.searchParams.set("token", token);
  return new Promise<void>((resolve, reject) => {
    const upload = new tus.Upload(content, {
      endpoint: endpoint.toString(), chunkSize: tusSettings.chunkSize,
      retryDelays: [0, 1000, 3000, 5000, 10000, 20000], parallelUploads: 1,
      storeFingerprintForResuming: true,
      headers: password ? { "X-SHARE-PASSWORD": encodeURIComponent(password) } : {},
      onShouldRetry(error) { const status = error.originalResponse?.getStatus() || 0; return ![401, 403, 404, 409].includes(status); },
      onProgress(bytesUploaded) { onupload({ loaded: bytesUploaded }); },
      onError(error) { delete publicUploads[key]; if (error.message === "Upload aborted") return reject(error); const message = error instanceof tus.DetailedError && error.originalResponse ? error.originalResponse.getBody() || "Upload failed" : "Upload failed"; reject(new Error(message)); },
      onSuccess() { delete publicUploads[key]; resolve(); },
    });
    publicUploads[key] = upload;
    upload.findPreviousUploads().then((previous) => { if (previous.length) upload.resumeFromPreviousUpload(previous[0]); upload.start(); }).catch(reject);
  });
}

export function abortTusUpload(hash: string, name: string, content: File) {
  const key = `${hash}:${name}:${content.size}:${content.lastModified}`;
  publicUploads[key]?.abort(true);
  delete publicUploads[key];
}
