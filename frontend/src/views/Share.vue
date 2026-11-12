<template>
  <div>
    <public-media-preview
      v-if="
        loadedShare &&
        !error &&
        !galleryClosed &&
        req &&
        !isUploadOnly &&
        !req.isDir &&
        ['image', 'video', 'audio'].includes(req.type)
      "
      :file="req"
      :password="password"
      v-model:original="originalImage"
      :selected="fileStore.gallerySelection?.paths.includes(req.path) ?? false"
      @toggle-selection="fileStore.toggleGallerySelection(route.path, req.path)"
      @close="closeGallery"
    />
    <header-bar showMenu showLogo>
      <title />
      <action
        v-if="
          loadedShare &&
          galleryClosed &&
          req &&
          !isUploadOnly &&
          !req.isDir &&
          ['image', 'video', 'audio'].includes(req.type)
        "
        icon="fullscreen"
        :label="t('sidebar.preview')"
        @action="galleryClosed = false"
      />

      <action
        v-if="fileStore.selectedCount && !isUploadOnly"
        icon="file_download"
        :label="t('buttons.download')"
        @action="download"
        :counter="fileStore.selectedCount"
      />
      <button
        v-if="isSingleFile() && !isUploadOnly"
        class="action copy-clipboard"
        :aria-label="t('buttons.copyDownloadLinkToClipboard')"
        :data-title="t('buttons.copyDownloadLinkToClipboard')"
        @click="copyToClipboard(linkSelected())"
      >
        <i class="material-icons">content_paste</i>
      </button>
      <action
        v-if="req?.isDir && req.allowUpload"
        icon="upload"
        :label="t('buttons.upload')"
        @action="openUpload"
      />
      <select-all-button v-if="canSwitchView" public-share />
      <action
        v-if="canSwitchView"
        :icon="listingViewModes[viewMode].icon"
        :label="t('buttons.switchView')"
        @action="switchView"
      />
      <template v-if="canSwitchView" #actions>
        <action
          icon="check_circle"
          :label="t('buttons.selectMultiple')"
          @action="toggleMultipleSelection"
        />
      </template>
    </header-bar>

    <breadcrumbs :base="'/share/' + hash" />
    <input
      ref="uploadInput"
      type="file"
      multiple
      hidden
      @change="uploadFiles"
    />

    <div v-if="layoutStore.loading">
      <h2 class="message delayed" style="padding-top: 3em !important">
        <div class="spinner">
          <div class="bounce1"></div>
          <div class="bounce2"></div>
          <div class="bounce3"></div>
        </div>
        <span>{{ t("files.loading") }}</span>
      </h2>
    </div>
    <div v-else-if="error">
      <div v-if="error.status === 401">
        <div class="card floating" id="password" style="z-index: 9999999">
          <div v-if="attemptedPasswordLogin" class="share__wrong__password">
            {{ t("login.wrongCredentials") }}
          </div>
          <div class="card-title">
            <h2>{{ t("login.password") }}</h2>
          </div>

          <div class="card-content">
            <input
              v-focus
              class="input input--block"
              type="password"
              :placeholder="t('login.password')"
              v-model="password"
              @keyup.enter="fetchData"
            />
          </div>
          <div class="card-action">
            <button
              class="button button--flat"
              @click="fetchData"
              :aria-label="t('buttons.submit')"
              :data-title="t('buttons.submit')"
            >
              {{ t("buttons.submit") }}
            </button>
          </div>
        </div>
        <div class="overlay" />
      </div>
      <errors v-else :errorCode="error.status" />
    </div>
    <div v-else-if="req !== null">
      <div class="share">
        <div v-if="!isUploadOnly" class="share__box share__box__info">
          <div class="share__box__header" style="height: 3em">
            {{
              req.isDir
                ? t("download.downloadFolder")
                : t("download.downloadFile")
            }}
          </div>
          <div
            v-if="!req.isDir"
            class="share__box__element share__box__center share__box__icon"
          >
            <i class="material-icons">{{ icon }}</i>
          </div>
          <div class="share__box__element" style="height: 3em">
            <strong>{{ $t("prompts.displayName") }}</strong> {{ req.name }}
          </div>
          <div v-if="!req.isDir" class="share__box__element" :title="modTime">
            <strong>{{ $t("prompts.lastModified") }}:</strong> {{ humanTime }}
          </div>
          <div
            v-if="req.isDir"
            class="share__box__element share-folder-summary"
          >
            <share-folder-size
              :key="req.url"
              :url="req.url"
              :password="password"
              :token="token"
            />
          </div>
          <div v-else class="share__box__element" style="height: 3em">
            <strong>{{ t("prompts.size") }}:</strong> {{ humanSize }}
          </div>
          <div v-if="req.isDir" class="share__box__element share-download-qr">
            <button
              type="button"
              class="button share-download-cta"
              @click="downloadFolder"
            >
              <i class="material-icons" aria-hidden="true">file_download</i>
              {{ t("buttons.download") }}
            </button>
            <qrcode-vue :value="link" :size="100" level="M" />
          </div>
          <div v-else class="share__box__element share__box__center">
            <a
              :href="link"
              class="button share-download-cta"
              referrerpolicy="no-referrer"
              download
            >
              <i class="material-icons" aria-hidden="true">file_download</i>
              {{ t("buttons.download") }}
            </a>
            <a
              target="_blank"
              :href="inlineLink"
              class="button button--flat"
              rel="noopener noreferrer"
            >
              <div>
                <i class="material-icons">open_in_new</i
                >{{ t("buttons.openFile") }}
              </div>
            </a>
          </div>
          <div v-if="!req.isDir" class="share__box__element share__box__center">
            <qrcode-vue :value="link" :size="200" level="M"></qrcode-vue>
          </div>
        </div>
        <div v-else class="share__box share__box__info upload-only-info">
          <div class="share__box__header">{{ t("buttons.upload") }}</div>
          <div class="share__box__element">
            {{ t("prompts.uploadOnlyNotice") }}
          </div>
        </div>
        <div
          id="shareList"
          v-if="req.isDir && req.items.length > 0"
          class="share__box share__box__items"
        >
          <div class="share__box__header share-list-header" v-if="req.isDir">
            <span>{{ t("files.files") }}</span>
            <span class="share-list-stats" aria-live="polite">
              <span v-if="req.numFiles > 0" class="share-list-stat">
                <i class="material-icons" aria-hidden="true"
                  >insert_drive_file</i
                >
                {{ req.numFiles }} {{ t("files.files") }}
              </span>
              <span v-if="req.numDirs > 0" class="share-list-stat">
                <i class="material-icons" aria-hidden="true">folder</i>
                {{ req.numDirs }} {{ t("files.folders") }}
              </span>
            </span>
          </div>
          <div
            v-if="!isUploadOnly"
            id="listing"
            class="file-icons responsive-listing"
            :class="viewMode"
          >
            <div>
              <item
                v-for="item in req.items.slice(0, showLimit)"
                :key="base64(item.name)"
                v-bind:index="item.index"
                v-bind:name="item.name"
                v-bind:isDir="item.isDir"
                v-bind:url="item.url"
                v-bind:modified="item.modified"
                v-bind:type="item.type"
                v-bind:size="item.size"
                :public-thumbnail-url="
                  req.showThumbnails && !isUploadOnly && item.type === 'image'
                    ? api.getThumbnailURL(hash, item.path, item.modified, token)
                    : undefined
                "
                :show-file-page-link="viewMode === 'list'"
                readOnly
              />
              <div
                v-if="req.items.length > showLimit"
                class="item"
                @click="showLimit += 100"
              >
                <div>
                  <p class="name">+ {{ req.items.length - showLimit }}</p>
                </div>
              </div>
            </div>

            <div
              :class="{ active: fileStore.multiple }"
              id="multiple-selection"
            >
              <p>{{ t("files.multipleSelectionEnabled") }}</p>
              <div
                @click="clearShareSelection"
                @keydown.enter.prevent="clearShareSelection"
                @keydown.space.prevent="clearShareSelection"
                tabindex="0"
                role="button"
                :data-title="t('buttons.clear')"
                :aria-label="t('buttons.clear')"
                class="action"
              >
                <i class="material-icons">clear</i>
              </div>
            </div>
          </div>
          <div v-else id="listing" class="list file-icons upload-only-list">
            <div
              v-for="item in req.items.slice(0, showLimit)"
              :key="base64(item.name)"
              class="item upload-only-item"
            >
              <div><i class="material-icons">insert_drive_file</i></div>
              <div>
                <p class="name">{{ item.name }}</p>
                <p class="size">{{ filesize(item.size) }}</p>
                <p class="modified">
                  <time :datetime="item.modified">{{
                    dayjs(item.modified).fromNow()
                  }}</time>
                </p>
              </div>
            </div>
            <div
              v-if="req.items.length > showLimit"
              class="item"
              @click="showLimit += 100"
            >
              <div>
                <p class="name">+ {{ req.items.length - showLimit }}</p>
              </div>
            </div>
          </div>
        </div>
        <div
          v-else-if="req.isDir && req.items.length === 0"
          class="share__box share__box__items"
        >
          <h2 class="message">
            <i class="material-icons">sentiment_dissatisfied</i>
            <span>{{ t("files.lonely") }}</span>
          </h2>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { pub as api } from "@/api";
import { filesize } from "@/utils";
import { listingViewModes } from "@/utils/listing-view-modes";
import dayjs from "dayjs";
import { Base64 } from "js-base64";
import HeaderBar from "@/components/header/HeaderBar.vue";
import Action from "@/components/header/Action.vue";
import SelectAllButton from "@/components/header/SelectAllButton.vue";
import Breadcrumbs from "@/components/Breadcrumbs.vue";
import Errors from "@/views/Errors.vue";
import QrcodeVue from "qrcode.vue";
import Item from "@/components/files/ListingItem.vue";
import PublicMediaPreview from "@/components/files/PublicMediaPreview.vue";
import ShareFolderSize from "@/components/files/ShareFolderSize.vue";
import urlUtils from "@/utils/url";
import { closeGalleryRoute } from "@/utils/gallery-history";
import { name as appName } from "@/utils/constants";
import { useFileStore } from "@/stores/file";
import { useLayoutStore } from "@/stores/layout";
import { computed, inject, onMounted, onBeforeUnmount, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { StatusError } from "@/api/utils";
import { copy } from "@/utils/clipboard";
import { useUploadStore } from "@/stores/upload";

const error = ref<StatusError | null>(null);
const showLimit = ref<number>(100);
const password = ref<string>("");
const attemptedPasswordLogin = ref<boolean>(false);
const hash = ref<string>("");
const token = ref<string>("");
const uploadInput = ref<HTMLInputElement>();
const originalImage = ref(false);
const viewMode = ref<ViewModeType>("list");
const galleryClosed = ref(false);
const loadedShare = ref(false);
let fetchController: AbortController | null = null;
let fetchGeneration = 0;
let disposed = false;

const $showError = inject<IToastError>("$showError")!;
const $showSuccess = inject<IToastSuccess>("$showSuccess")!;

const { t } = useI18n({});

const route = useRoute();
const router = useRouter();
const fileStore = useFileStore();
const layoutStore = useLayoutStore();
const uploadStore = useUploadStore();

watch(route, () => {
  showLimit.value = 100;
  fetchData();
});

const req = computed(() => fileStore.req);
const isUploadOnly = computed(() => Boolean(req.value?.uploadOnly));
const canSwitchView = computed(
  () =>
    loadedShare.value &&
    req.value?.isDir &&
    !isUploadOnly.value &&
    !error.value &&
    !layoutStore.loading
);

function switchView() {
  layoutStore.closeHovers();
  viewMode.value = listingViewModes[viewMode.value].next;
}

// Define computes

const icon = computed(() => {
  if (req.value === null) return "insert_drive_file";
  if (req.value.isDir) return "folder";
  if (req.value.type === "image") return "insert_photo";
  if (req.value.type === "audio") return "volume_up";
  if (req.value.type === "video") return "movie";
  return "insert_drive_file";
});

const link = computed(() => (req.value ? api.getDownloadURL(req.value) : ""));
const inlineLink = computed(() =>
  req.value ? api.getDownloadURL(req.value, true) : ""
);
const humanSize = computed(() => {
  if (req.value) {
    return req.value.isDir
      ? req.value.items.length
      : filesize(req.value.size ?? 0);
  } else {
    return "";
  }
});
const humanTime = computed(() => dayjs(req.value?.modified).fromNow());
const modTime = computed(() =>
  req.value
    ? new Date(Date.parse(req.value.modified)).toLocaleString()
    : new Date().toLocaleString()
);

// Functions
const base64 = (name: any) => Base64.encodeURI(name);
const fetchData = async () => {
  if (disposed) return;
  const generation = ++fetchGeneration;
  fetchController?.abort();
  const controller = new AbortController();
  fetchController = controller;
  const routePath = route.path;
  const parts = route.params.path;
  const shareHash = Array.isArray(parts)
    ? parts[0]
    : String(parts || "").split("/")[0];
  if (hash.value && hash.value !== shareHash) {
    password.value = "";
    token.value = "";
    originalImage.value = false;
    attemptedPasswordLogin.value = false;
    loadedShare.value = false;
  }
  hash.value = shareHash || "";
  fileStore.reload = false;
  fileStore.selected = [];
  fileStore.multiple = false;
  layoutStore.closeHovers();

  // Set loading to true and reset the error.
  layoutStore.loading = true;
  error.value = null;
  if (password.value !== "") {
    attemptedPasswordLogin.value = true;
  }

  let url = route.path;
  if (url === "") url = "/";
  if (url[0] !== "/") url = "/" + url;

  try {
    const file = await api.fetch(url, password.value, controller.signal);
    if (disposed || generation !== fetchGeneration || route.path !== routePath)
      return;
    file.hash = shareHash;

    token.value = file.token || "";

    fileStore.updateRequest(file);
    if (fileStore.restoreGallerySelection(route.path)) {
      const firstSelected = fileStore.selected[0];
      if (firstSelected !== undefined) {
        showLimit.value = Math.max(showLimit.value, firstSelected + 1);
      }
    }
    galleryClosed.value = route.query.view === "file";
    loadedShare.value = true;
    document.title = `${file.name} - ${t("buttons.share")} - ${appName}`;
  } catch (err) {
    if (disposed || generation !== fetchGeneration || controller.signal.aborted)
      return;
    if (err instanceof Error) {
      error.value = err;
    }
  } finally {
    if (!disposed && generation === fetchGeneration) {
      layoutStore.loading = false;
      fetchController = null;
    }
  }
};

function closeGallery() {
  galleryClosed.value = true;
  if (route.path.split("/").filter(Boolean).length > 2) {
    void closeGalleryRoute(router, urlUtils.removeLastDir(route.path)).catch(
      (error) => $showError(error instanceof Error ? error : String(error))
    );
  }
}

const clearShareSelection = () => {
  fileStore.selected = [];
  fileStore.multiple = false;
};

const keyEvent = (event: KeyboardEvent) => {
  if (event.key === "Escape") clearShareSelection();
};

const toggleMultipleSelection = () => {
  layoutStore.closeHovers();
  if (fileStore.multiple) clearShareSelection();
  else fileStore.toggleMultiple();
};

const openUpload = () => uploadInput.value?.click();

const uploadFiles = async (event: Event) => {
  const selectedFiles = (event.target as HTMLInputElement).files;
  if (!selectedFiles || !hash.value) return;

  for (const file of Array.from(selectedFiles)) {
    uploadStore.upload("/", file.name, file, false, "blob", {
      hash: hash.value,
      token: token.value,
      password: password.value,
    });
  }
  (event.target as HTMLInputElement).value = "";
};

watch(
  () => fileStore.reload,
  (reload) => {
    if (reload && route.name === "Share") fetchData();
  }
);

watch(
  () => uploadStore.publicUploadRevision,
  (revision) => {
    if (revision > 0 && route.name === "Share") fetchData();
  }
);

const isSingleFile = () => {
  const item = req.value?.items?.[fileStore.selected[0]];
  return fileStore.selectedCount === 1 && !!item && !item.isDir;
};

const download = () => {
  const share = req.value;
  if (!share?.isDir || share.uploadOnly || layoutStore.loading) return;
  const selected = fileStore.selected
    .map((index) => share.items[index])
    .filter(Boolean);
  if (!selected.length) return;

  const paths = selected.map((item) => item.path);
  const urls = paths.map((path) =>
    api.getDownloadURL({ ...share, hash: hash.value, token: token.value, path })
  );
  const files = selected.filter((item) => !item.isDir);
  const includesFolders = files.length !== selected.length;
  const selectedBytes = files.length
    ? files.reduce((total, item) => total + item.size, 0)
    : undefined;
  if (selected.length === 1 && !includesFolders) {
    api.startDownload(urls[0]);
    return;
  }

  const sharePassword = password.value;
  const archiveURL = api.getZipDownloadURL(hash.value, token.value, paths);
  openShareDownload({
    archiveURL,
    zipChecks: [...urls, archiveURL],
    individualURLs: async () => urls,
    includesFolders,
    fileCount: includesFolders ? undefined : files.length,
    selectedBytes: includesFolders ? undefined : selectedBytes,
    calculateTotals: includesFolders
      ? (signal) => api.getZipDownloadTotals(archiveURL, sharePassword, signal)
      : undefined,
  });
};

const downloadFolder = () => {
  const share = req.value;
  if (!share?.isDir || share.uploadOnly || layoutStore.loading) return;
  const sharePassword = password.value;
  openShareDownload({
    archiveURL: api.getFolderArchiveURL(share),
    zipChecks: [api.getDownloadURL(share)],
    showSubfolderNote: share.numDirs > 0,
    individualURLs: async (signal) => {
      const plan = await api.prepareFolderDownload(
        share,
        sharePassword,
        signal
      );
      return plan.paths.map((path) => api.getDownloadURL({ ...share, path }));
    },
  });
};

function openShareDownload({
  archiveURL,
  zipChecks,
  individualURLs,
  includesFolders = false,
  showSubfolderNote = false,
  fileCount,
  calculateTotals,
  selectedBytes,
}: {
  archiveURL: string;
  zipChecks: string[];
  individualURLs: (signal: AbortSignal) => Promise<string[]>;
  includesFolders?: boolean;
  showSubfolderNote?: boolean;
  fileCount?: number;
  calculateTotals?: (
    signal: AbortSignal
  ) => Promise<{ fileCount: number; size: number }>;
  selectedBytes?: number;
}) {
  const openedOn = route.fullPath;
  layoutStore.showHover({
    prompt: "share-download",
    props: {
      archiveURL,
      includesFolders,
      showSubfolderNote,
      fileCount,
      calculateTotals,
      selectedBytes,
    },
    confirm: async (mode: "all" | "zip", signal: AbortSignal) => {
      if (mode === "all" && includesFolders) return;
      const downloads =
        mode === "zip" ? [archiveURL] : await individualURLs(signal);
      if (signal.aborted || route.fullPath !== openedOn) return;
      if (downloads.length === 0) return false;
      await api.checkDownloads(mode === "zip" ? zipChecks : downloads, signal);
      if (signal.aborted || route.fullPath !== openedOn) return;
      for (const url of downloads) api.startDownload(url);
    },
  });
}

const linkSelected = () => {
  return isSingleFile() && req.value
    ? api.getDownloadURL({
        ...req.value,
        hash: hash.value,
        path: req.value.items[fileStore.selected[0]].path,
      })
    : "";
};

const copyToClipboard = (text: string) => {
  copy({ text }).then(
    () => {
      // clipboard successfully set
      $showSuccess(t("success.linkCopied"));
    },
    () => {
      // clipboard write failed
      copy({ text }, { permission: true }).then(
        () => {
          // clipboard successfully set
          $showSuccess(t("success.linkCopied"));
        },
        (e) => {
          // clipboard write failed
          $showError(e);
        }
      );
    }
  );
};

onMounted(async () => {
  // Created
  window.addEventListener("keydown", keyEvent);
  await fetchData();
});

onBeforeUnmount(() => {
  disposed = true;
  fileStore.gallerySelection = null;
  fetchGeneration++;
  fetchController?.abort();
  // Destroyed
  window.removeEventListener("keydown", keyEvent);
});
</script>

<style scoped>
.share-folder-summary {
  padding-block: 0.45rem;
  word-break: normal;
}

.share-download-qr {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 1rem;
}

.share__box__element .share-download-cta {
  display: inline-flex;
  min-height: 2.5rem;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  padding: 0.35rem 0.8rem;
  border-radius: 0.45rem;
  font-size: 0.9rem;
  font-weight: 500;
}

.share-download-cta:hover {
  color: #fff;
}

.share__box__element .share-download-cta .material-icons {
  display: inline-block;
  margin: 0;
  font-size: 1.2rem;
}

#listing.list {
  height: auto;
}

#shareList {
  overflow-y: auto;
}

#shareList #listing.mosaic {
  margin: 0;
}

.share-list-header {
  align-items: center;
  border-bottom: 1px solid var(--borderPrimary);
  display: flex;
  gap: 0.75rem;
  justify-content: space-between;
  position: sticky;
  top: 0;
  z-index: 2;
  background: var(--surfacePrimary);
}

.share-list-stats {
  align-items: center;
  color: var(--textSecondary);
  display: inline-flex;
  flex-wrap: wrap;
  font-size: 0.78em;
  font-weight: normal;
  gap: 0.6rem;
  justify-content: flex-end;
}

.share-list-stat {
  align-items: center;
  display: inline-flex;
  gap: 0.22rem;
  white-space: nowrap;
}

.share-list-stat .material-icons {
  font-size: 1.05em;
}

@media (max-width: 736px) {
  .share__box__element .share-download-cta {
    min-height: 2.75rem;
  }
}

@media (max-width: 420px) {
  .share-list-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .share-list-stats {
    justify-content: flex-start;
  }
}

@media (min-width: 930px) {
  #shareList {
    height: calc(100vh - 9.8em);
    overflow-y: auto;
  }
}
</style>
