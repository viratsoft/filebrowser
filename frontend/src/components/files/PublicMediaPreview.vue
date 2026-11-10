<template>
  <div
    id="previewer"
    :class="{ 'image-gallery': file.type === 'image' }"
    @touchmove.prevent.stop
    @wheel.prevent.stop
    @pointermove="revealVideoControls"
  >
    <header-bar v-if="showControls">
      <action
        icon="close"
        :label="t('buttons.close')"
        @action="emit('close')"
      />
      <title>{{ file.name }}</title>
      <button
        v-if="file.type === 'image' && file.allowImagePreview"
        type="button"
        class="action"
        :disabled="layout.loading"
        :aria-pressed="effectiveOriginal"
        :title="qualityLabel"
        :aria-label="qualityLabel"
        @click="toggleQuality"
      >
        <i class="material-icons" aria-hidden="true">{{
          effectiveOriginal ? "photo_size_select_large" : "hd"
        }}</i>
      </button>
    </header-bar>
    <div
      class="preview"
      :class="{ 'image-gallery-viewport': file.type === 'image' }"
    >
      <extended-image
        v-if="file.type === 'image' && source"
        :key="file.path"
        :src="source"
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
        <i v-else class="material-icons spin" :aria-label="t('files.loading')"
          >autorenew</i
        >
      </div>
    </div>
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
import HeaderBar from "@/components/header/HeaderBar.vue";
import Action from "@/components/header/Action.vue";
import ExtendedImage from "./ExtendedImage.vue";
import VideoPlayer from "./VideoPlayer.vue";

const props = defineProps<{
  file: Resource;
  password: string;
  original: boolean;
}>();
const emit = defineEmits<{
  (event: "close"): void;
  (event: "update:original", original: boolean): void;
}>();
const router = useRouter();
const layout = useLayoutStore();
const { t } = useI18n();
const showError = inject<IToastError>("$showError");
const showControls = ref(true);
const navigating = ref(false);
const source = ref("");
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
const qualityLabel = computed(() =>
  t(
    effectiveOriginal.value
      ? "buttons.previewQuality"
      : "buttons.originalQuality"
  )
);
const mediaURL = computed(() =>
  props.file.uploadOnly
    ? ""
    : props.file.type !== "image" || effectiveOriginal.value
      ? api.getDownloadURL(props.file, true)
      : api.getImagePreviewURL(props.file)
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

function toggleQuality() {
  if (
    !layout.loading &&
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
  if (fileURL.split("/").filter(Boolean).length <= 2) {
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
  navigating.value = true;
  try {
    await router.push({ path: item.url });
  } catch (error) {
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
  if (event.key === "Escape") emit("close");
  else if (props.file.type !== "video" && event.key === "ArrowLeft")
    void go(previous.value);
  else if (props.file.type !== "video" && event.key === "ArrowRight")
    void go(next.value);
}

watch(
  () => [props.file.hash, props.file.url, props.file.uploadOnly],
  () => {
    void loadSiblings();
  },
  { immediate: true }
);
watch(
  () => [mediaURL.value, props.password, props.file.type],
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
