<template>
  <div
    class="gallery-stage"
    :class="{
      'is-changing': changing,
      'going-back': backward,
      'is-dragging': dragging,
      'media-swipe-enabled': swipeMedia,
    }"
    :style="{ '--slide-drag': dragOffset + 'px' }"
    @touchstart.capture="startSwipe"
    @touchmove.capture="moveSwipe"
    @touchend.capture="endSwipe"
    @touchcancel.capture="resetSwipe"
  >
    <slot />
  </div>
  <div v-if="changing" class="gallery-slide-loading">
    <gallery-spinner :label="t('files.loading')" />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import GallerySpinner from "./GallerySpinner.vue";

const props = defineProps<{
  changing: boolean;
  backward: boolean;
  swipeMedia?: boolean;
  canGoNext?: boolean;
  canGoPrevious?: boolean;
}>();
const emit = defineEmits<{ (event: "next" | "previous"): void }>();
const { t } = useI18n();
const dragOffset = ref(0);
const dragging = ref(false);
let start: { id: number; x: number; y: number } | undefined;
let settleTimer: ReturnType<typeof setTimeout> | undefined;

function resetSwipe() {
  start = undefined;
  dragging.value = false;
  dragOffset.value = 0;
  if (settleTimer !== undefined) clearTimeout(settleTimer);
  settleTimer = undefined;
}

function startSwipe(event: TouchEvent) {
  resetSwipe();
  if (!props.swipeMedia || props.changing || event.touches.length !== 1) return;
  if (
    event.target instanceof Element &&
    event.target.closest(
      "audio, button, a, input, [role=slider], .vjs-control-bar, .vjs-menu, .vjs-fullscreen"
    )
  )
    return;
  const touch = event.touches[0];
  start = { id: touch.identifier, x: touch.clientX, y: touch.clientY };
}

function moveSwipe(event: TouchEvent) {
  if (!start) return;
  const touch = event.touches[0];
  if (event.touches.length !== 1 || touch.identifier !== start.id) {
    resetSwipe();
    return;
  }
  const dx = touch.clientX - start.x;
  const dy = touch.clientY - start.y;
  if (!dragging.value && Math.max(Math.abs(dx), Math.abs(dy)) < 12) return;
  if (!dragging.value && Math.abs(dx) < Math.abs(dy) * 1.25) {
    resetSwipe();
    return;
  }
  dragging.value = true;
  if (event.cancelable) event.preventDefault();
  event.stopPropagation();
  const canMove = dx < 0 ? props.canGoNext : props.canGoPrevious;
  dragOffset.value = canMove ? Math.max(-100, Math.min(100, dx * 0.6)) : 0;
}

function endSwipe(event: TouchEvent) {
  if (!start) return;
  const touch = Array.from(event.changedTouches).find(
    (entry) => entry.identifier === start?.id
  );
  if (!touch) return;
  const dx = touch.clientX - start.x;
  const dy = touch.clientY - start.y;
  const horizontal = Math.abs(dx) >= 45 && Math.abs(dx) >= Math.abs(dy) * 1.25;
  const canMove = dx < 0 ? props.canGoNext : props.canGoPrevious;
  if (horizontal || dragging.value) {
    if (event.cancelable) event.preventDefault();
    event.stopPropagation();
  }
  if (!horizontal || !canMove || props.changing) {
    resetSwipe();
    return;
  }
  start = undefined;
  dragging.value = false;
  emit(dx < 0 ? "next" : "previous");
  // Restore the stage if navigation was interrupted or rejected.
  settleTimer = setTimeout(resetSwipe, 230);
}

watch(
  () => props.changing,
  (changing) => {
    if (!changing) resetSwipe();
  }
);
watch(() => props.swipeMedia, resetSwipe);
onBeforeUnmount(resetSwipe);
</script>

<style scoped>
.gallery-stage {
  width: 100%;
  height: 100%;
  transition:
    opacity 180ms ease-out,
    transform 180ms cubic-bezier(0.2, 0.72, 0.22, 1);
}
.gallery-stage.media-swipe-enabled {
  touch-action: pan-y pinch-zoom;
}
.gallery-stage.is-dragging {
  transition: none;
  transform: translate3d(var(--slide-drag), 0, 0);
}
.gallery-stage.is-changing {
  opacity: 0;
  transform: translate3d(calc(var(--slide-drag) - 22px), 0, 0);
  pointer-events: none;
}
.gallery-stage.is-changing.going-back {
  transform: translate3d(calc(var(--slide-drag) + 22px), 0, 0);
}
.gallery-slide-loading {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  color: #fff;
  pointer-events: none;
  animation: show-slide-loader 0s 80ms both;
}
@keyframes show-slide-loader {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}
@media (prefers-reduced-motion: reduce) {
  .gallery-stage {
    transition: none;
  }
  .gallery-stage.is-dragging,
  .gallery-stage.is-changing {
    transform: none;
  }
}
</style>
