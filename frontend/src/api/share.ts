import { fetchURL, fetchJSON, removePrefix, createURL } from "./utils";

export async function list() {
  return fetchJSON<Share[]>("/api/shares");
}

export async function get(url: string) {
  url = removePrefix(url);
  return fetchJSON<Share>(`/api/share${url}`);
}

export async function remove(hash: string) {
  await fetchURL(`/api/share/${hash}`, {
    method: "DELETE",
  });
}

export async function create(
  url: string,
  password = "",
  expires = "",
  unit = "hours",
  allowUpload = false,
  uploadOnly = false,
  name = ""
) {
  url = removePrefix(url);
  url = `/api/share${url}`;
  if (expires !== "") {
    url += `?expires=${expires}&unit=${unit}`;
  }
  let body = "{}";
  if (password != "" || expires !== "" || unit !== "hours" || allowUpload || uploadOnly || name) {
    body = JSON.stringify({
      password: password,
      expires: expires.toString(), // backend expects string not number
      unit: unit,
      allowUpload,
      uploadOnly,
      name,
    });
  }
  return fetchJSON(url, {
    method: "POST",
    body: body,
  });
}

export function getShareURL(share: Share) {
  return createURL("share/" + share.hash, {});
}
