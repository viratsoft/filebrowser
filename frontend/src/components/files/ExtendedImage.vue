<template>
  <div
    ref="container"
    class="image-ex-container"
    :class="[
      ...props.classList,
      {
        'gallery-zoomed': transform.scale > 1,
        'gallery-dragging': interacting,
        'gallery-animate': animate,
      },
    ]"
    @pointerdown="pointerDown"
    @lostpointercapture="pointerCancel"
    @touchstart="touchStart"
    @touchmove="touchMove"
    @touchend="touchEnd"
    @touchcancel="cancelGesture"
    @mousedown="mouseDown"
    @wheel.prevent="wheel"
    @dragstart.prevent
  >
    <img
      :key="generation"
      ref="imgex"
      class="image-ex-img"
      :alt="props.alt"
      :style="imageStyle"
      :aria-hidden="!ready"
      draggable="false"
      @load="onLoad"
      @error="onError"
    />
    <div v-if="!ready" class="gallery-status" role="status">
      <span v-if="failed">{{ t("files.noPreview") }}</span>
      <i v-else class="material-icons spin" :aria-label="t('files.loading')"
        >autorenew</i
      >
    </div>
  </div>
</template>

<script setup lang="ts">
import UTIF from "utif";
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  shallowRef,
  watch,
} from "vue";
import { useI18n } from "vue-i18n";
import { ImageGalleryController } from "@/utils/image-gallery";
import type {
  GalleryNavigation,
  GalleryPointerKind,
} from "@/utils/image-gallery";

const props = withDefaults(
  defineProps<{
    src: string;
    alt?: string;
    classList?: string[];
    zoomStep?: number;
    moveDisabledTime?: number;
    canGoNext?: boolean;
    canGoPrevious?: boolean;
  }>(),
  {
    alt: "",
    classList: () => [],
    zoomStep: 0.25,
    canGoNext: false,
    canGoPrevious: false,
  }
);
const emit = defineEmits<{ (event: "next" | "previous" | "tap"): void }>();
const { t } = useI18n();
const model = new ImageGalleryController();
const transform = shallowRef(model.transform);
const container = ref<HTMLDivElement | null>(null);
const imgex = ref<HTMLImageElement | null>(null);
const ready = ref(false);
const failed = ref(false);
const animate = ref(false);
const interacting = ref(false);
const generation = ref(0);
const pointerSupported =
  typeof window !== "undefined" && "PointerEvent" in window;
let observer: ResizeObserver | null = null;
let xhr: XMLHttpRequest | null = null;
let disposed = false;
let decodeAttempted = false;
let lastLegacyTouch = -Infinity;
let tapTimer: ReturnType<typeof setTimeout> | null = null;
// Wall-clock corrections must not stretch the double-tap window or its timer.
const now = () => window.performance.now();

function clearTapTimer() {
  if (tapTimer !== null) clearTimeout(tapTimer);
  tapTimer = null;
}
function scheduleSingleTap() {
  clearTapTimer();
  const delay = model.singleTapDelay(now());
  if (delay === null) return;
  tapTimer = setTimeout(() => {
    tapTimer = null;
    if (disposed) return;
    if (model.consumeSingleTap(now())) emit("tap");
    else scheduleSingleTap();
  }, delay);
}

const imageStyle = computed(() => ({
  width: `${transform.value.width}px`,
  height: `${transform.value.height}px`,
  transform: `translate3d(${transform.value.left}px, ${transform.value.top}px, 0) scale(${transform.value.scale})`,
  opacity: ready.value ? 1 : 0,
}));
const publish = () => {
  transform.value = model.transform;
  interacting.value = model.isInteracting;
};
const local = (event: { clientX: number; clientY: number }) => {
  const bounds = container.value?.getBoundingClientRect();
  return {
    x: event.clientX - (bounds?.left ?? 0),
    y: event.clientY - (bounds?.top ?? 0),
  };
};
const prevent = (event: Event) => {
  if (event.cancelable) event.preventDefault();
};
const kind = (type: string): GalleryPointerKind =>
  type === "mouse" ? "mouse" : type === "pen" ? "pen" : "touch";
const navigate = (action: GalleryNavigation) => {
  if (action === "next" && props.canGoNext) emit("next");
  if (action === "previous" && props.canGoPrevious) emit("previous");
};

function pointerDown(event: PointerEvent) {
  if (
    !pointerSupported ||
    (event.pointerType === "mouse" && event.button !== 0)
  )
    return;
  if (
    !model.pointerDown(
      event.pointerId,
      local(event),
      now(),
      kind(event.pointerType)
    )
  )
    return;
  animate.value = false;
  clearTapTimer();
  prevent(event);
  try {
    container.value?.setPointerCapture(event.pointerId);
  } catch {
    /* Older browser capture support is optional. */
  }
  publish();
}
function pointerMove(event: PointerEvent) {
  if (!pointerSupported || !model.hasPointer(event.pointerId)) return;
  prevent(event);
  model.pointerMove(event.pointerId, local(event));
  publish();
}
function pointerUp(event: PointerEvent) {
  if (!pointerSupported || !model.hasPointer(event.pointerId)) return;
  prevent(event);
  const action = model.pointerUp(event.pointerId, local(event), now());
  animate.value = !model.isInteracting;
  publish();
  navigate(action);
  scheduleSingleTap();
}
function pointerCancel(event: PointerEvent) {
  if (pointerSupported && model.hasPointer(event.pointerId)) cancelGesture();
}
function cancelGesture() {
  clearTapTimer();
  model.cancelGesture();
  animate.value = true;
  publish();
}

// Touch/mouse fallback for browsers without Pointer Events. Never bind both
// paths to one gesture, and ignore synthetic mouse events following a touch.
function touchStart(event: TouchEvent) {
  if (pointerSupported) return;
  lastLegacyTouch = now();
  clearTapTimer();
  prevent(event);
  animate.value = false;
  for (const touch of Array.from(event.changedTouches))
    model.pointerDown(touch.identifier, local(touch), now(), "touch");
  publish();
}
function touchMove(event: TouchEvent) {
  if (pointerSupported) return;
  lastLegacyTouch = now();
  prevent(event);
  for (const touch of Array.from(event.changedTouches))
    model.pointerMove(touch.identifier, local(touch));
  publish();
}
function touchEnd(event: TouchEvent) {
  if (pointerSupported) return;
  lastLegacyTouch = now();
  prevent(event);
  let action: GalleryNavigation = null;
  for (const touch of Array.from(event.changedTouches))
    action = model.pointerUp(touch.identifier, local(touch), now()) || action;
  animate.value = !model.isInteracting;
  publish();
  navigate(action);
  scheduleSingleTap();
}
function mouseDown(event: MouseEvent) {
  if (pointerSupported || event.button !== 0 || now() - lastLegacyTouch < 800)
    return;
  prevent(event);
  clearTapTimer();
  animate.value = false;
  model.pointerDown(-1, local(event), now(), "mouse");
  publish();
}
function mouseMove(event: MouseEvent) {
  if (pointerSupported || !model.hasPointer(-1)) return;
  prevent(event);
  model.pointerMove(-1, local(event));
  publish();
}
function mouseUp(event: MouseEvent) {
  if (pointerSupported || !model.hasPointer(-1)) return;
  prevent(event);
  const before = model.scale;
  model.pointerUp(-1, local(event), now());
  animate.value = before !== model.scale;
  publish();
  scheduleSingleTap();
}
function wheel(event: WheelEvent) {
  if (!ready.value || model.isInteracting || !Number.isFinite(event.deltaY))
    return;
  prevent(event);
  clearTapTimer();
  animate.value = false;
  const delta =
    event.deltaY *
    (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? 100 : 1);
  const step =
    Number.isFinite(props.zoomStep) && props.zoomStep > 0
      ? props.zoomStep
      : 0.25;
  model.zoomAt(
    local(event),
    model.scale * Math.exp(Math.max(-1, Math.min(1, (-delta * step) / 100)))
  );
  publish();
}
function resize() {
  if (!container.value || disposed) return;
  animate.value = false;
  model.setViewport(container.value.clientWidth, container.value.clientHeight);
  publish();
}
function onLoad(event: Event) {
  if (disposed || event.target !== imgex.value || !imgex.value) return;
  model.setImage(imgex.value.naturalWidth, imgex.value.naturalHeight);
  resize();
  ready.value = model.fitted.width > 0;
  failed.value = !ready.value;
}
function onError(event?: Event) {
  if (disposed || (event && event.target !== imgex.value)) return;
  // A preview endpoint may return JPEG/PNG even when its path ends in .tif.
  // Give native decoding first refusal, then use the existing RAW/TIFF fallback.
  let extension = "";
  try {
    extension =
      new URL(props.src, document.baseURI).pathname
        .split(".")
        .pop()
        ?.toLowerCase() || "";
    if (props.src.startsWith("blob:"))
      extension = props.alt.split(".").pop()?.toLowerCase() || "";
  } catch {
    /* A malformed URL follows the normal image error path. */
  }
  if (
    !decodeAttempted &&
    ["tif", "tiff", "dng", "cr2", "nef"].includes(extension)
  ) {
    decodeAttempted = true;
    decodeImage();
    return;
  }
  ready.value = false;
  failed.value = true;
  cancelGesture();
}
function visibilityChanged() {
  if (document.hidden) cancelGesture();
}

async function loadImage() {
  clearTapTimer();
  const current = ++generation.value;
  xhr?.abort();
  xhr = null;
  ready.value = false;
  failed.value = false;
  decodeAttempted = false;
  animate.value = false;
  model.setImage(0, 0);
  publish();
  await nextTick();
  if (disposed || current !== generation.value || !imgex.value) return;
  imgex.value.src = props.src;
}

function decodeImage() {
  const current = generation.value;
  const request = new XMLHttpRequest();
  xhr = request;
  try {
    request.open("GET", props.src);
    request.responseType = "arraybuffer";
  } catch {
    xhr = null;
    onError();
    return;
  }
  request.onload = (event) => {
    if (disposed || current !== generation.value || !imgex.value) return;
    if (xhr === request) xhr = null;
    if (
      request.status !== 0 &&
      (request.status < 200 || request.status >= 300)
    ) {
      onError();
      return;
    }
    // Preserve UTIF's existing TIFF/RAW page-selection behavior, but register
    // only a current response so a stale decode cannot overwrite another slide.
    UTIF._xhrs.push(request);
    UTIF._imgs.push(imgex.value);
    try {
      UTIF._imgLoaded(event);
    } catch {
      onError();
    } finally {
      const index = UTIF._xhrs.indexOf(request);
      if (index !== -1) {
        UTIF._xhrs.splice(index, 1);
        UTIF._imgs.splice(index, 1);
      }
      if (xhr === request) xhr = null;
    }
  };
  request.onerror = () => {
    if (xhr === request) xhr = null;
    if (current === generation.value && !disposed) onError();
  };
  try {
    request.send();
  } catch {
    if (xhr === request) xhr = null;
    if (current === generation.value && !disposed) onError();
  }
}

watch(() => props.src, loadImage);
onMounted(() => {
  resize();
  loadImage();
  if (typeof ResizeObserver !== "undefined") {
    observer = new ResizeObserver(resize);
    observer.observe(container.value!);
  }
  window.addEventListener("resize", resize);
  window.addEventListener("pointermove", pointerMove);
  window.addEventListener("pointerup", pointerUp);
  window.addEventListener("pointercancel", pointerCancel);
  window.addEventListener("blur", cancelGesture);
  document.addEventListener("visibilitychange", visibilityChanged);
  window.addEventListener("mousemove", mouseMove);
  window.addEventListener("mouseup", mouseUp);
});
onBeforeUnmount(() => {
  disposed = true;
  clearTapTimer();
  generation.value++;
  xhr?.abort();
  observer?.disconnect();
  model.cancelGesture();
  window.removeEventListener("resize", resize);
  window.removeEventListener("pointermove", pointerMove);
  window.removeEventListener("pointerup", pointerUp);
  window.removeEventListener("pointercancel", pointerCancel);
  window.removeEventListener("blur", cancelGesture);
  document.removeEventListener("visibilitychange", visibilityChanged);
  window.removeEventListener("mousemove", mouseMove);
  window.removeEventListener("mouseup", mouseUp);
});
</script>

<style scoped>
.image-ex-container {
  width: 100%;
  height: 100%;
  overflow: hidden;
  position: relative;
  touch-action: none;
  overscroll-behavior: contain;
  user-select: none;
  -webkit-user-select: none;
}
.image-ex-img {
  position: absolute;
  left: 0;
  top: 0;
  margin: 0;
  transform-origin: 0 0;
  user-select: none;
  -webkit-user-drag: none;
}
.gallery-zoomed {
  cursor: grab;
}
.gallery-zoomed.gallery-dragging {
  cursor: grabbing;
}
.gallery-animate .image-ex-img {
  transition: transform 160ms ease-out;
}
.gallery-status {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  pointer-events: none;
}
@media (prefers-reduced-motion: reduce) {
  .gallery-animate .image-ex-img {
    transition: none;
  }
}
</style>
