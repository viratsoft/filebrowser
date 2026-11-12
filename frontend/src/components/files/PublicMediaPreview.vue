<template>
  <div
    id="previewer"
    class="media-gallery"
    :class="{ 'image-gallery': file.type === 'image' }"
    @touchmove.prevent.stop
    @wheel.prevent.stop
    @pointermove="revealVideoControls"
    @contextmenu.capture.prevent.stop
  >
    <header-bar v-if="showControls" class="gallery-toolbar">
      <action
        icon="close"
        :label="t('buttons.close')"
        @action="emit('close')"
      />
      <gallery-title
        :filename="file.name"
        :position="singleFileShare ? 1 : currentIndex + 1"
        :total="singleFileShare ? 1 : siblings.length"
        :bytes="file.size"
      />
      <a
        v-if="downloadURL"
        class="action"
        :class="{ disabled: downloadDisabled }"
        :href="downloadDisabled ? undefined : downloadURL"
        :aria-disabled="downloadDisabled"
        :aria-label="t('buttons.download')"
        :title="t('buttons.download')"
        referrerpolicy="no-referrer"
        download
      >
        <i class="material-icons" aria-hidden="true">file_download</i>
      </a>
      <template #trailing>
        <gallery-selection-button
          v-if="!singleFileShare"
          :selected="selected ?? false"
          :count="selectedCount ?? 0"
          :disabled="layout.loading || navigating"
          @toggle="emit('toggle-selection')"
        />
      </template>
    </header-bar>
    <div
      class="preview"
      :class="{ 'image-gallery-viewport': file.type === 'image' }"
    >
      <gallery-stage
        :changing="changingSlide"
        :backward="backward"
        :swipe-media="file.type === 'video' || file.type === 'audio'"
        :can-go-next="!!next && !navigating && !layout.loading"
        :can-go-previous="!!previous && !navigating && !layout.loading"
        @next="go(next)"
        @previous="go(previous)"
      >
        <extended-image
          v-if="file.type === 'image' && source"
          :key="file.path"
          :src="source"
          preserve-swipe
          :alt="file.name"
          :can-go-next="!!next && !navigating && !layout.loading"
          :can-go-previous="!!previous && !navigating && !layout.loading"
          @tap="showControls = !showControls"
          @next="go(next)"
          @previous="go(previous)"
        />
        <video-player
          v-else-if="file.type === 'video'"
          :key="file.path"
          :source="mediaURL"
          :options="{ autoplay: false }"
        />
        <audio
          v-else-if="file.type === 'audio'"
          :key="file.path"
          :src="mediaURL"
          controls
        />
        <div v-else class="public-media-status" role="status">
          <span v-if="failed">{{ t("files.noPreview") }}</span>
          <gallery-spinner v-else :label="t('files.loading')" />
        </div>
      </gallery-stage>
    </div>
    <gallery-quality-button
      v-if="showControls && file.type === 'image' && file.allowImagePreview"
      :original="effectiveOriginal"
      :disabled="layout.loading || navigating"
      @toggle="toggleQuality"
    />
    <button
      :class="{ hidden: !previous || !showControls }"
      :disabled="navigating || layout.loading"
      :aria-label="t('buttons.previous')"
      @click="go(previous)"
    >
      <i class="material-icons">chevron_left</i>
    </button>
    <button
      :class="{ hidden: !next || !showControls }"
      :disabled="navigating || layout.loading"
      :aria-label="t('buttons.next')"
      @click="go(next)"
    >
      <i class="material-icons">chevron_right</i>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { pub as api } from "@/api";
import { useLayoutStore } from "@/stores/layout";
import url from "@/utils/url";
import { consumeGallerySelectionShortcut } from "@/utils/gallery-keyboard";
import HeaderBar from "@/components/header/HeaderBar.vue";
import Action from "@/components/header/Action.vue";
import ExtendedImage from "./ExtendedImage.vue";
import VideoPlayer from "./VideoPlayer.vue";
import GallerySpinner from "./GallerySpinner.vue";
import GalleryStage from "./GalleryStage.vue";
import GalleryTitle from "./GalleryTitle.vue";
import GalleryQualityButton from "./GalleryQualityButton.vue";
import GallerySelectionButton from "./GallerySelectionButton.vue";

const props = defineProps<{
  file: Resource;
  password: string;
  original: boolean;
  selected?: boolean;
  selectedCount?: number;
}>();
const emit = defineEmits<{
  (event: "close"): void;
  (event: "update:original", original: boolean): void;
  (event: "toggle-selection"): void;
}>();
const router = useRouter();
const layout = useLayoutStore();
const { t } = useI18n();
const showError = inject<IToastError>("$showError");
const showControls = ref(true);
const navigating = ref(false);
const source = ref("");
const pendingPath = ref("");
const backward = ref(false);
const failed = ref(false);
const siblings = ref<ResourceItem[]>([]);
let parentURL = "";
let listingRequest = 0;
let imageRequest = 0;
let listingController: AbortController | null = null;
let imageController: AbortController | null = null;
let objectURL = "";
let controlsTimer: ReturnType<typeof setTimeout> | null = null;
let disposed = false;

const effectiveOriginal = computed(
  () => props.original || !props.file.allowImagePreview
);
const downloadDisabled = computed(() => layout.loading || navigating.value);
const downloadURL = computed(() =>
  !props.file.uploadOnly &&
  !props.file.isDir &&
  (!props.password || props.file.token)
    ? api.getDownloadURL(props.file)
    : ""
);
const mediaURL = computed(() =>
  props.file.uploadOnly
    ? ""
    : props.file.type !== "image" || effectiveOriginal.value
      ? api.getDownloadURL(props.file, true)
      : api.getImagePreviewURL(props.file)
);
const singleFileShare = computed(
  () => props.file.url.split("/").filter(Boolean).length <= 2
);
const currentIndex = computed(() =>
  siblings.value.findIndex((item) => item.url === props.file.url)
);
const previous = computed(() =>
  currentIndex.value > 0 ? siblings.value[currentIndex.value - 1] : undefined
);
const next = computed(() =>
  currentIndex.value >= 0 ? siblings.value[currentIndex.value + 1] : undefined
);
const changingSlide = computed(
  () =>
    navigating.value ||
    (layout.loading &&
      !!pendingPath.value &&
      props.file.url !== pendingPath.value)
);

function toggleQuality() {
  if (
    !layout.loading &&
    !navigating.value &&
    props.file.type === "image" &&
    props.file.allowImagePreview &&
    !props.file.uploadOnly
  )
    emit("update:original", !effectiveOriginal.value);
}

async function loadSiblings() {
  const current = ++listingRequest;
  listingController?.abort();
  listingController = null;
  const fileURL = props.file.url;
  const hash = props.file.hash;
  const parent = url.removeLastDir(fileURL);
  if (props.file.uploadOnly) {
    siblings.value = [];
    parentURL = "";
    return;
  }
  if (singleFileShare.value) {
    siblings.value = [];
    parentURL = "";
    return;
  }
  if (parentURL === parent && siblings.value.length) return;
  parentURL = parent;
  siblings.value = [];
  const controller = new AbortController();
  listingController = controller;
  try {
    const listing = await api.fetch(
      parent,
      props.password,
      controller.signal,
      props.file.token
    );
    if (
      disposed ||
      current !== listingRequest ||
      hash !== props.file.hash ||
      parent !== parentURL
    )
      return;
    if (listing.isDir && !listing.uploadOnly)
      siblings.value = listing.items.filter(
        (item) => !item.isDir && ["image", "video", "audio"].includes(item.type)
      );
  } catch {
    /* The current image remains usable if sibling listing is unavailable. */
  } finally {
    if (listingController === controller) listingController = null;
  }
}

function clearObjectURL() {
  if (objectURL) URL.revokeObjectURL(objectURL);
  objectURL = "";
}
async function loadSource() {
  const current = ++imageRequest;
  imageController?.abort();
  imageController = null;
  clearObjectURL();
  source.value = "";
  failed.value = false;
  if (props.file.type !== "image" || props.file.uploadOnly) return;
  // Ordinary shares use native image streaming. Only legacy password shares
  // without a bearer token need a header-authenticated object URL.
  if (!props.password || props.file.token) {
    source.value = mediaURL.value;
    return;
  }
  const controller = new AbortController();
  imageController = controller;
  try {
    const blob = await api.fetchImage(
      props.file,
      effectiveOriginal.value,
      props.password,
      controller.signal
    );
    if (disposed || current !== imageRequest || controller.signal.aborted)
      return;
    objectURL = URL.createObjectURL(blob);
    source.value = objectURL;
  } catch {
    if (!disposed && current === imageRequest && !controller.signal.aborted)
      failed.value = true;
  } finally {
    if (imageController === controller) imageController = null;
  }
}

async function go(item?: ResourceItem) {
  if (!item || navigating.value || disposed || layout.loading) return;
  pendingPath.value = item.url;
  backward.value = item.url === previous.value?.url;
  navigating.value = true;
  try {
    // The folder-to-gallery navigation owns the history entry for every slide.
    await router.replace({ path: item.url });
  } catch (error) {
    pendingPath.value = "";
    showError?.(error instanceof Error ? error : String(error));
  } finally {
    navigating.value = false;
  }
}
function revealVideoControls() {
  if (props.file.type === "image") return;
  showControls.value = true;
  if (controlsTimer !== null) clearTimeout(controlsTimer);
  controlsTimer = setTimeout(() => {
    showControls.value = false;
    controlsTimer = null;
  }, 1500);
}
function key(event: KeyboardEvent) {
  if (layout.currentPrompt !== null) return;
  if (
    props.file.type === "image" &&
    !props.file.uploadOnly &&
    !singleFileShare.value &&
    !layout.loading &&
    !navigating.value &&
    consumeGallerySelectionShortcut(event)
  ) {
    showControls.value = true;
    emit("toggle-selection");
    return;
  }
  if (event.key === "Escape") emit("close");
  else if (props.file.type !== "video" && event.key === "ArrowLeft")
    void go(previous.value);
  else if (props.file.type !== "video" && event.key === "ArrowRight")
    void go(next.value);
}

watch(
  () => [
    props.file.hash,
    props.file.url,
    props.file.uploadOnly,
    props.file.token,
    props.password,
  ],
  (current, previous) => {
    if (
      previous &&
      (current[0] !== previous[0] ||
        current[3] !== previous[3] ||
        current[4] !== previous[4])
    ) {
      parentURL = "";
      siblings.value = [];
    }
    void loadSiblings();
  },
  { immediate: true }
);
watch(
  () => [props.file, mediaURL.value, props.password],
  () => {
    void loadSource();
  },
  { immediate: true }
);
watch(
  () => props.file.type,
  (type, previousType) => {
    if (controlsTimer !== null) clearTimeout(controlsTimer);
    controlsTimer = null;
    if (type !== previousType) showControls.value = true;
  }
);
watch(
  () => layout.loading,
  (loading) => {
    if (!loading) pendingPath.value = "";
  }
);
onMounted(() => window.addEventListener("keydown", key));
onBeforeUnmount(() => {
  disposed = true;
  listingRequest++;
  imageRequest++;
  listingController?.abort();
  imageController?.abort();
  clearObjectURL();
  if (controlsTimer !== null) clearTimeout(controlsTimer);
  window.removeEventListener("keydown", key);
});
</script>

<style scoped>
#previewer {
  background-color: #000;
}
.public-media-status {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  pointer-events: none;
}
</style>
