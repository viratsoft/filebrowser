<template>
  <div
    id="previewer"
    :class="{ 'image-gallery': fileStore.req?.type === 'image' }"
    @touchmove.prevent.stop
    @wheel.prevent.stop
    @mousemove="revealNavigation"
    @touchstart="revealNavigation"
  >
    <header-bar
      v-if="isPdf || isEpub || isCsv || showNav"
      :class="{ 'gallery-toolbar': isGalleryMedia }"
    >
      <action icon="close" :label="$t('buttons.close')" @action="close()" />
      <GalleryTitle
        v-if="isGalleryMedia"
        :filename="name"
        :position="galleryPosition"
        :total="galleryItems.length"
        :bytes="layoutStore.loading ? undefined : fileStore.req?.size"
      />
      <title v-else>{{ name }}</title>
      <template #actions>
        <action
          :disabled="layoutStore.loading"
          v-if="authStore.user?.perm.rename"
          icon="mode_edit"
          :label="$t('buttons.rename')"
          show="rename"
        />
        <action
          :disabled="layoutStore.loading"
          v-if="isCsv && authStore.user?.perm.modify"
          icon="edit_note"
          :label="t('buttons.editAsText')"
          @action="editAsText"
        />
        <action
          :disabled="layoutStore.loading"
          v-if="authStore.user?.perm.delete"
          icon="delete"
          :label="$t('buttons.delete')"
          @action="deleteFile"
          id="delete-button"
        />
        <action
          :disabled="layoutStore.loading"
          v-if="authStore.user?.perm.download"
          icon="file_download"
          :label="$t('buttons.download')"
          @action="download"
        />
        <action
          :disabled="layoutStore.loading"
          v-if="
            ['image', 'audio', 'video'].includes(fileStore.req?.type || '') &&
            authStore.user?.perm.download
          "
          icon="open_in_new"
          :label="t('buttons.openDirect')"
          @action="openDirect"
        />
        <action
          :disabled="layoutStore.loading"
          icon="info"
          :label="$t('buttons.info')"
          show="info"
        />
      </template>
      <template #trailing>
        <GallerySelectionButton
          v-if="isGalleryMedia && fileStore.req"
          :selected="
            fileStore.gallerySelection?.paths.includes(fileStore.req.path) ??
            false
          "
          :disabled="layoutStore.loading || navigationPending"
          @toggle="
            fileStore.toggleGallerySelection(route.path, fileStore.req.path)
          "
        />
      </template>
    </header-bar>

    <div class="loading delayed" v-if="layoutStore.loading && !changingSlide">
      <div class="spinner">
        <div class="bounce1"></div>
        <div class="bounce2"></div>
        <div class="bounce3"></div>
      </div>
    </div>
    <template v-else>
      <div
        class="preview"
        :class="{ 'image-gallery-viewport': fileStore.req?.type === 'image' }"
      >
        <GalleryStage
          :changing="changingSlide"
          :backward="backward"
          :swipe-media="
            fileStore.req?.type === 'video' || fileStore.req?.type === 'audio'
          "
          :can-go-next="hasNext && !layoutStore.loading && !navigationPending"
          :can-go-previous="
            hasPrevious && !layoutStore.loading && !navigationPending
          "
          @next="next"
          @previous="prev"
        >
          <div v-if="isEpub" class="epub-reader">
            <vue-reader
              :location="location"
              :url="previewUrl"
              :get-rendition="getRendition"
              :epubInitOptions="{
                requestCredentials: true,
              }"
              :epubOptions="{
                allowPopups: true,
              }"
              @update:location="locationChange"
            />
            <div class="size">
              <button
                @click="changeSize(Math.max(100, size - 10))"
                class="reader-button"
              >
                <i class="material-icons">remove</i>
              </button>
              <button
                @click="changeSize(Math.min(150, size + 10))"
                class="reader-button"
              >
                <i class="material-icons">add</i>
              </button>
              <span>{{ size }}%</span>
            </div>
          </div>
          <CsvViewer
            v-else-if="isCsv"
            :content="csvContent"
            :error="csvError"
          />
          <ExtendedImage
            v-else-if="fileStore.req?.type == 'image'"
            :key="fileStore.req?.path"
            :src="previewUrl"
            :alt="name"
            preserve-swipe
            :can-go-next="hasNext && !layoutStore.loading && !navigationPending"
            :can-go-previous="
              hasPrevious && !layoutStore.loading && !navigationPending
            "
            @next="next"
            @previous="prev"
            @tap="toggleGalleryNavigation"
          />
          <audio
            v-else-if="fileStore.req?.type == 'audio'"
            ref="audioPlayer"
            :src="previewUrl"
            controls
            :autoplay="autoPlay"
            @play="autoPlay = true"
          ></audio>
          <VideoPlayer
            v-else-if="fileStore.req?.type == 'video'"
            :key="fileStore.req.path"
            ref="videoPlayer"
            :source="previewUrl"
            :subtitles="subtitles"
            :options="videoOptions"
          >
          </VideoPlayer>
          <object v-else-if="isPdf" class="pdf" :data="previewUrl"></object>
          <div v-else-if="fileStore.req?.type == 'blob'" class="info">
            <div class="title">
              <i class="material-icons">feedback</i>
              {{ $t("files.noPreview") }}
            </div>
            <div>
              <a
                target="_blank"
                :href="downloadUrl"
                class="button button--flat"
              >
                <div>
                  <i class="material-icons">file_download</i
                  >{{ $t("buttons.download") }}
                </div>
              </a>
              <a
                target="_blank"
                :href="previewUrl"
                class="button button--flat"
                v-if="!fileStore.req?.isDir"
              >
                <div>
                  <i class="material-icons">open_in_new</i
                  >{{ $t("buttons.openFile") }}
                </div>
              </a>
            </div>
          </div>
        </GalleryStage>
      </div>
    </template>

    <GalleryQualityButton
      v-if="showNav && isResizeEnabled && fileStore.req?.type === 'image'"
      :disabled="layoutStore.loading || navigationPending"
      :original="fullSize"
      @toggle="toggleSize"
    />
    <button
      @click="prev"
      @mouseover="hoverNav = true"
      @mouseleave="hoverNav = false"
      :class="{ hidden: !hasPrevious || !showNav }"
      :disabled="navigationPending || layoutStore.loading"
      :aria-label="$t('buttons.previous')"
      :title="$t('buttons.previous')"
    >
      <i class="material-icons">chevron_left</i>
    </button>
    <button
      @click="next"
      @mouseover="hoverNav = true"
      @mouseleave="hoverNav = false"
      :class="{ hidden: !hasNext || !showNav }"
      :disabled="navigationPending || layoutStore.loading"
      :aria-label="$t('buttons.next')"
      :title="$t('buttons.next')"
    >
      <i class="material-icons">chevron_right</i>
    </button>
  </div>
</template>

<script setup lang="ts">
import { useStorage } from "@vueuse/core";
import { useAuthStore } from "@/stores/auth";
import { useFileStore } from "@/stores/file";
import { useLayoutStore } from "@/stores/layout";

import { files as api } from "@/api";
import { createURL } from "@/api/utils";
import { resizePreview } from "@/utils/constants";
import url from "@/utils/url";
import { closeGalleryRoute } from "@/utils/gallery-history";
import { throttle } from "lodash-es";
import HeaderBar from "@/components/header/HeaderBar.vue";
import Action from "@/components/header/Action.vue";
import ExtendedImage from "@/components/files/ExtendedImage.vue";
import GalleryStage from "@/components/files/GalleryStage.vue";
import GalleryTitle from "@/components/files/GalleryTitle.vue";
import GalleryQualityButton from "@/components/files/GalleryQualityButton.vue";
import GallerySelectionButton from "@/components/files/GallerySelectionButton.vue";
import VideoPlayer from "@/components/files/VideoPlayer.vue";
import CsvViewer from "@/components/files/CsvViewer.vue";
import { VueReader } from "vue-reader";
import { computed, inject, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import type { Rendition } from "epubjs";
import { getTheme } from "@/utils/theme";
import { useI18n } from "vue-i18n";

// CSV file size limit for preview (5MB)
// Prevents browser memory issues with large files
const CSV_MAX_SIZE = 5 * 1024 * 1024;

const location = useStorage("book-progress", 0, undefined, {
  serializer: {
    read: (v) => JSON.parse(v),
    write: (v) => JSON.stringify(v),
  },
});
const size = useStorage("book-size", 120, undefined, {
  serializer: {
    read: (v) => JSON.parse(v),
    write: (v) => JSON.stringify(v),
  },
});

const locationChange = (epubcifi: number) => {
  location.value = epubcifi;
};
let rendition: Rendition | null = null;
const changeSize = (val: number) => {
  size.value = val;
  rendition?.themes.fontSize(`${val}%`);
};

const getRendition = (_rendition: Rendition) => {
  rendition = _rendition;
  switch (getTheme()) {
    case "dark": {
      rendition.themes.override("color", "rgba(255, 255, 255, 0.6)");
      break;
    }
    case "light": {
      rendition.themes.override("color", "rgb(111, 111, 111)");
      break;
    }
  }
  rendition.themes.registerRules("h2Transparent", {
    "h1,h2,h3,h4": {
      "background-color": "transparent !important",
    },
  });
  rendition?.themes.fontSize(`${size.value}%`);
  rendition.themes.select("h2Transparent");
  rendition.themes.override("background-color", "transparent", true);
};

const galleryTypes: ResourceType[] = ["image", "video", "audio"];
const previewTypes: ResourceType[] = [...galleryTypes, "blob"];

const previousLink = ref<string>("");
const nextLink = ref<string>("");
const listing = ref<ResourceItem[] | null>(null);
let listingFolder = "";
let listingController: AbortController | null = null;
let previewUpdate = 0;
let disposed = false;
const name = ref<string>("");
const fullSize = ref<boolean>(false);
const showNav = ref<boolean>(true);
const navTimeout = ref<null | number>(null);
const hoverNav = ref<boolean>(false);
const autoPlay = ref<boolean>(false);
const csvContent = ref<ArrayBuffer | string>("");
const csvError = ref<string>("");

const audioPlayer = ref<HTMLAudioElement | null>(null);
const videoPlayer = ref<InstanceType<typeof VideoPlayer> | null>(null);

const $showError = inject<IToastError>("$showError")!;

const authStore = useAuthStore();
const fileStore = useFileStore();
const layoutStore = useLayoutStore();

const { t } = useI18n();

const route = useRoute();
const router = useRouter();

const isGalleryMedia = computed(
  () => !!fileStore.req && galleryTypes.includes(fileStore.req.type)
);
const galleryItems = computed(() =>
  (listing.value ?? []).filter((item) => galleryTypes.includes(item.type))
);
const galleryPosition = computed(
  () => galleryItems.value.findIndex((item) => item.name === name.value) + 1
);

const hasPrevious = computed(() => previousLink.value !== "");

const hasNext = computed(() => nextLink.value !== "");
const navigationPending = ref(false);
const pendingPath = ref("");
const backward = ref(false);
const changingSlide = computed(
  () =>
    isGalleryMedia.value &&
    (navigationPending.value ||
      (layoutStore.loading && pendingPath.value === route.path))
);

const downloadUrl = computed(() =>
  fileStore.req ? api.getDownloadURL(fileStore.req, false) : ""
);

const directUrl = computed(() =>
  fileStore.req ? api.getDownloadURL(fileStore.req, true) : ""
);

const previewUrl = computed(() => {
  if (!fileStore.req) {
    return "";
  }

  if (fileStore.req.type === "image" && resizePreview && !fullSize.value) {
    return api.getPreviewURL(fileStore.req, "big");
  }

  if (isEpub.value) {
    return createURL("api/raw" + fileStore.req.path, {});
  }

  return api.getDownloadURL(fileStore.req, true);
});

const isPdf = computed(() => fileStore.req?.extension.toLowerCase() == ".pdf");
const isEpub = computed(
  () => fileStore.req?.extension.toLowerCase() == ".epub"
);
const isCsv = computed(
  () =>
    fileStore.req?.extension.toLowerCase() == ".csv" &&
    fileStore.req.size <= CSV_MAX_SIZE
);

const isResizeEnabled = computed(() => resizePreview);

const subtitles = computed(() => {
  if (fileStore.req?.subtitles) {
    return api.getSubtitlesURL(fileStore.req);
  }
  return [];
});

const videoOptions = computed(() => {
  return { autoplay: autoPlay.value };
});

watch(route, () => {
  updatePreview();
});

// Resource metadata arrives after the route changes. Use its actual media type
// so image tap controls and video auto-hide never inherit each other's timers.
watch(
  () => fileStore.req?.type,
  (type, previous) => {
    if (type === "image") {
      clearNavigationTimer();
      hoverNav.value = false;
      if (previous !== "image") showNav.value = true;
    } else toggleNavigation();
  }
);

// Specify hooks
onMounted(async () => {
  window.addEventListener("keydown", key);
  listingFolder = url.removeLastDir(route.path);
  const previousFolder = fileStore.oldReq;
  listing.value =
    previousFolder?.isDir &&
    previousFolder.url.replace(/\/+$/, "") === listingFolder
      ? previousFolder.items
      : null;
  updatePreview();
});

onBeforeUnmount(() => {
  disposed = true;
  previewUpdate++;
  listingController?.abort();
  window.removeEventListener("keydown", key);
  if (navTimeout.value !== null) clearTimeout(navTimeout.value);
  toggleNavigation.cancel();
});

// Specify methods
const deleteFile = () => {
  layoutStore.showHover({
    prompt: "delete",
    confirm: () => {
      if (listing.value === null) {
        return;
      }

      const index = listing.value.findIndex((item) => item.name == name.value);
      listing.value.splice(index, 1);

      if (hasNext.value) {
        next();
      } else if (!hasPrevious.value && !hasNext.value) {
        const nearbyItem = listing.value[Math.max(0, index - 1)];
        fileStore.preselect = nearbyItem?.path;

        close();
      } else {
        prev();
      }
    },
  });
};

const navigateTo = async (path: string) => {
  if (!path || layoutStore.loading || navigationPending.value) return;
  pendingPath.value = path;
  backward.value = path === previousLink.value;
  navigationPending.value = true;
  hoverNav.value = false;
  try {
    await router.replace({ path });
  } catch (error) {
    $showError(error instanceof Error ? error : String(error));
  } finally {
    navigationPending.value = false;
  }
};

const prev = () => navigateTo(previousLink.value);

const next = () => navigateTo(nextLink.value);

const key = (event: KeyboardEvent) => {
  if (layoutStore.currentPrompt !== null) {
    return;
  }
  // When previewing a video, let arrow keys fall through to video.js for
  // seeking instead of switching to the prev/next file. Enter still advances.
  const isVideo = fileStore.req?.type === "video";
  if (event.which === 13) {
    // enter
    if (hasNext.value) next();
  } else if (event.which === 39) {
    // right arrow
    if (isVideo) return;
    if (hasNext.value) next();
  } else if (event.which === 37) {
    // left arrow
    if (isVideo) return;
    if (hasPrevious.value) prev();
  } else if (event.which === 27) {
    // esc
    close();
  }
};
const updatePreview = async () => {
  if (disposed) return;
  const update = ++previewUpdate;
  const currentPath = route.path;
  const parent = url.removeLastDir(currentPath);
  listingController?.abort();
  listingController = null;
  if (listingFolder !== parent) {
    listingFolder = parent;
    listing.value = null;
  }
  previousLink.value = "";
  nextLink.value = "";
  if (
    (audioPlayer.value?.paused && !audioPlayer.value.ended) ||
    videoPlayer.value?.isPausedBeforeEnd()
  ) {
    autoPlay.value = false;
  }

  const dirs = currentPath.split("/");
  name.value = decodeURIComponent(dirs[dirs.length - 1]);

  // Load CSV content if it's a CSV file
  if (isCsv.value && fileStore.req) {
    csvContent.value = "";
    csvError.value = "";

    if (fileStore.req.size > CSV_MAX_SIZE) {
      csvError.value = t("files.csvTooLarge");
    } else {
      if (fileStore.req.rawContent != null) {
        csvContent.value = fileStore.req.rawContent;
      } else {
        csvContent.value = fileStore.req.content ?? "";
      }
    }
  }

  if (!listing.value) {
    const controller = new AbortController();
    listingController = controller;
    try {
      const res = await api.fetch(parent, controller.signal);
      if (disposed || update !== previewUpdate || currentPath !== route.path)
        return;
      listing.value = res.items;
    } catch (error) {
      if (!disposed && update === previewUpdate && !controller.signal.aborted) {
        $showError(error instanceof Error ? error : String(error));
      }
      return;
    } finally {
      if (listingController === controller) listingController = null;
    }
  }

  if (listing.value) {
    for (let i = 0; i < listing.value.length; i++) {
      if (listing.value[i].name !== name.value) {
        continue;
      }

      const mediaTypes = galleryTypes.includes(listing.value[i].type)
        ? galleryTypes
        : previewTypes;
      for (let j = i - 1; j >= 0; j--) {
        if (mediaTypes.includes(listing.value[j].type)) {
          previousLink.value = listing.value[j].url;
          break;
        }
      }
      for (let j = i + 1; j < listing.value.length; j++) {
        if (mediaTypes.includes(listing.value[j].type)) {
          nextLink.value = listing.value[j].url;
          break;
        }
      }

      return;
    }
  }
};

const toggleSize = () => {
  if (!layoutStore.loading && !navigationPending.value) {
    fullSize.value = !fullSize.value;
  }
};

const toggleNavigation = throttle(function () {
  showNav.value = true;

  if (navTimeout.value) {
    clearTimeout(navTimeout.value);
  }

  navTimeout.value = window.setTimeout(() => {
    showNav.value = false || hoverNav.value;
    navTimeout.value = null;
  }, 1500);
}, 500);

// Image gestures decide whether a completed action was a tap. Revealing the
// toolbar on touchstart/mousemove would undo a tap-to-hide or interrupt a pinch.
const clearNavigationTimer = () => {
  toggleNavigation.cancel();
  if (navTimeout.value !== null) clearTimeout(navTimeout.value);
  navTimeout.value = null;
};
const revealNavigation = () => {
  if (fileStore.req?.type !== "image") toggleNavigation();
};
const toggleGalleryNavigation = () => {
  if (fileStore.req?.type !== "image") return;
  clearNavigationTimer();
  hoverNav.value = false;
  showNav.value = !showNav.value;
};

const close = () => {
  const uri = url.removeLastDir(route.path) + "/";
  if (isGalleryMedia.value) {
    void closeGalleryRoute(router, uri).catch((error) =>
      $showError(error instanceof Error ? error : String(error))
    );
  } else {
    void router.push({ path: uri });
  }
};

const download = () => window.open(downloadUrl.value);
const openDirect = () => window.open(directUrl.value);

const editAsText = () => {
  router.push({ path: route.path, query: { edit: "true" } });
};
</script>
