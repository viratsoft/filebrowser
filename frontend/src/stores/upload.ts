import { defineStore } from "pinia";
import { useFileStore } from "./file";
import { files as api, pub as publicApi } from "@/api";
import buttons from "@/utils/buttons";
import { computed, inject, markRaw, ref } from "vue";
import * as tus from "@/api/tus";

// TODO: make this into a user setting
const UPLOADS_LIMIT = 5;
// Public share uploads are intentionally limited below the normal five-file
// queue. Three concurrent resumable streams use available bandwidth well
// without overwhelming slow disks, reverse proxies, or mobile connections.
const PUBLIC_UPLOADS_LIMIT = 3;

const beforeUnload = (event: Event) => {
  event.preventDefault();
  // To remove >> is deprecated
  // event.returnValue = "";
};

export const useUploadStore = defineStore("upload", () => {
  const $showError = inject<IToastError>("$showError")!;

  let progressInterval: number | null = null;

  //
  // STATE
  //

  const allUploads = ref<Upload[]>([]);
  const activeUploads = ref<Set<Upload>>(new Set());
  const lastUpload = ref<number>(-1);
  const totalBytes = ref<number>(0);
  const sentBytes = ref<number>(0);
  const publicUploadRevision = ref<number>(0);

  //
  // ACTIONS
  //

  const upload = (
    path: string,
    name: string,
    file: File | null,
    overwrite: boolean,
    type: ResourceType,
    publicShare?: Upload["publicShare"]
  ) => {
    if (!hasActiveUploads() && !hasPendingUploads()) {
      window.addEventListener("beforeunload", beforeUnload);
      buttons.loading("upload");
    }

    const upload: Upload = {
      path,
      name,
      file,
      overwrite,
      type,
      totalBytes: file?.size || 1,
      sentBytes: 0,
      // Stores rapidly changing sent bytes value without causing component re-renders
      rawProgress: markRaw({
        sentBytes: 0,
      }),
      publicShare,
    };

    totalBytes.value += upload.totalBytes;
    allUploads.value.push(upload);

    processUploads();
  };

  const abort = () => {
    // Resets the state by preventing the processing of the remaning uploads
    lastUpload.value = Infinity;
    for (const upload of activeUploads.value) {
      if (upload.publicShare && upload.file) {
        publicApi.abortTusUpload(upload.publicShare.hash, upload.name, upload.file);
      }
    }
    tus.abortAllUploads();
  };

  //
  // GETTERS
  //

  const pendingUploadCount = computed(
    () =>
      allUploads.value.length -
      (lastUpload.value + 1) +
      activeUploads.value.size
  );

  //
  // PRIVATE FUNCTIONS
  //

  const hasActiveUploads = () => activeUploads.value.size > 0;

  const hasPendingUploads = () =>
    allUploads.value.length > lastUpload.value + 1;

  const isActiveUploadsOnLimit = () => {
    if (activeUploads.value.size >= UPLOADS_LIMIT) return false;

    const next = allUploads.value[lastUpload.value + 1];
    if (!next?.publicShare) return true;

    let publicActiveUploads = 0;
    for (const active of activeUploads.value) {
      if (active.publicShare) publicActiveUploads++;
    }
    return publicActiveUploads < PUBLIC_UPLOADS_LIMIT;
  };

  const processUploads = async () => {
    if (!hasActiveUploads() && !hasPendingUploads()) {
      const fileStore = useFileStore();
      window.removeEventListener("beforeunload", beforeUnload);
      buttons.success("upload");
      reset();
      fileStore.reload = true;
    }

    if (isActiveUploadsOnLimit() && hasPendingUploads()) {
      if (progressInterval === null) {
        // Update the state in a fixed time interval. Guard on the handle, not
        // on the active count: this runs again every time the queue drains to
        // zero with work still pending, and reassigning would leak the timer.
        progressInterval = window.setInterval(syncState, 1000);
      }

      const upload = nextUpload();
      let succeeded = true;

      if (upload.type === "dir") {
        await api.post(upload.path).catch((err) => {
          succeeded = false;
          $showError(err);
        });
      } else {
        const onUpload = (event: { loaded: number }) => {
          upload.rawProgress.sentBytes = event.loaded;
        };

        const request = upload.publicShare
          ? publicApi.tusUpload(upload.publicShare.hash, upload.name, upload.file!, upload.publicShare.token, upload.publicShare.password, onUpload)
          : api.post(upload.path, upload.file!, upload.overwrite, onUpload);
        await request.catch((err) => {
          succeeded = false;
          if (err.message === "Upload aborted") return;

          // A public share never overwrites an existing file. Turn the
          // expected 409 response into a useful, local message without
          // exposing any server-side path information.
          if (upload.publicShare && /^409(?:\s|$)/.test(err.message)) {
            $showError(
              `File already exists: ${upload.name}`,
              false,
              "compact-conflict-toast"
            );
            return;
          }

          $showError(err);
        });
      }

      finishUpload(upload, succeeded);
    }
  };

  const nextUpload = (): Upload => {
    lastUpload.value++;

    const upload = allUploads.value[lastUpload.value];
    activeUploads.value.add(upload);

    return upload;
  };

  const finishUpload = (upload: Upload, succeeded: boolean) => {
    if (succeeded) {
      sentBytes.value += upload.totalBytes - upload.sentBytes;
      upload.sentBytes = upload.totalBytes;
    } else {
      // Credit only what actually reached the server and drop the rest from the
      // total. Counting a failed upload as fully sent makes the bar report 100%
      // while nothing was written, which reads as success.
      sentBytes.value += upload.rawProgress.sentBytes - upload.sentBytes;
      totalBytes.value -= upload.totalBytes - upload.rawProgress.sentBytes;
      upload.sentBytes = upload.rawProgress.sentBytes;
    }
    if (succeeded && upload.publicShare) {
      // Share.vue watches this revision and refreshes only the visitor's
      // listing after every completed file, while other queue items continue.
      publicUploadRevision.value++;
    }
    upload.file = null;

    activeUploads.value.delete(upload);
    processUploads();
  };

  const syncState = () => {
    for (const upload of activeUploads.value) {
      sentBytes.value += upload.rawProgress.sentBytes - upload.sentBytes;
      upload.sentBytes = upload.rawProgress.sentBytes;
    }
  };

  const reset = () => {
    if (progressInterval !== null) {
      clearInterval(progressInterval);
      progressInterval = null;
    }

    allUploads.value = [];
    activeUploads.value = new Set();
    lastUpload.value = -1;
    totalBytes.value = 0;
    sentBytes.value = 0;
  };

  return {
    // STATE
    activeUploads,
    totalBytes,
    sentBytes,
    publicUploadRevision,

    // ACTIONS
    upload,
    abort,

    // GETTERS
    pendingUploadCount,
  };
});
