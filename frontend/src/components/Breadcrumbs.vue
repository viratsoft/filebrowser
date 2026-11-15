<template>
  <div
    class="breadcrumbs"
    :class="{ 'fade-left': hiddenLeft, 'fade-right': hiddenRight }"
  >
    <div
      ref="viewport"
      class="breadcrumb-scroll"
      role="region"
      :aria-label="t('settings.path')"
      :tabindex="hiddenLeft || hiddenRight ? 0 : undefined"
      @scroll.passive="updateEdges"
      @wheel.passive="keepManualPosition"
      @touchstart.passive="keepManualPosition"
      @focusin="onPathFocus"
      @keydown.self="scrollWithKeyboard"
    >
      <div ref="track" class="breadcrumb-track">
        <component
          :is="element"
          class="breadcrumb-home"
          :to="base || ''"
          :aria-label="t('files.home')"
          :title="t('files.home')"
          :aria-current="items.length === 0 ? 'location' : undefined"
        >
          <i class="material-icons" aria-hidden="true">home</i>
        </component>

        <span
          v-for="(link, index) in items"
          :key="link.url"
          class="breadcrumb-item"
        >
          <span class="chevron" aria-hidden="true"
            ><i class="material-icons">keyboard_arrow_right</i></span
          >
          <component
            :is="element"
            class="breadcrumb-label"
            :to="link.url"
            :title="link.name"
            :aria-current="index === items.length - 1 ? 'location' : undefined"
          >
            {{ link.name }}
          </component>
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute } from "vue-router";

const { t, locale } = useI18n();

const route = useRoute();

const props = defineProps<{
  base: string;
  noLink?: boolean;
}>();

const viewport = ref<HTMLElement>();
const track = ref<HTMLElement>();
const hiddenLeft = ref(false);
const hiddenRight = ref(false);
let observer: ResizeObserver | undefined;
let alignOnResize = true;

function keepManualPosition() {
  alignOnResize = false;
}

function onPathFocus(event: FocusEvent) {
  if (event.target !== viewport.value) keepManualPosition();
}

function onPathResize() {
  if (alignOnResize) revealCurrent();
  else updateEdges();
}

function onLocationChange() {
  alignOnResize = true;
  revealCurrent();
}

function updateEdges() {
  if (!viewport.value || !track.value) return;
  if (viewport.value.scrollWidth <= viewport.value.clientWidth) {
    hiddenLeft.value = false;
    hiddenRight.value = false;
    return;
  }
  const visible = viewport.value.getBoundingClientRect();
  const content = track.value.getBoundingClientRect();
  hiddenLeft.value = content.left < visible.left - 1;
  hiddenRight.value = content.right > visible.right + 1;
}

function revealCurrent() {
  const scroller = viewport.value;
  const current = track.value?.lastElementChild;
  if (!scroller || !current) return;
  const visible = scroller.getBoundingClientRect();
  const item = current.getBoundingClientRect();
  const rtl = getComputedStyle(scroller).direction === "rtl";
  // Keep the start of an exceptionally long name visible and let it be swiped.
  const alignLeft = item.width <= visible.width ? rtl : !rtl;
  scroller.scrollLeft += alignLeft
    ? item.left - visible.left
    : item.right - visible.right;
  updateEdges();
}

function scrollWithKeyboard(event: KeyboardEvent) {
  const scroller = viewport.value;
  if (
    !scroller ||
    event.altKey ||
    event.ctrlKey ||
    event.metaKey ||
    event.shiftKey
  )
    return;
  if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
  if (!hiddenLeft.value && !hiddenRight.value) return;
  event.preventDefault();
  keepManualPosition();
  const rtl = getComputedStyle(scroller).direction === "rtl";
  if (event.key === "Home" || event.key === "End") {
    scroller.scrollLeft =
      event.key === "Home" ? 0 : scroller.scrollWidth * (rtl ? -1 : 1);
  } else {
    scroller.scrollLeft +=
      (scroller.clientWidth / 3) * (event.key === "ArrowLeft" ? -1 : 1);
  }
  updateEdges();
}

watch([() => route.path, () => props.base, locale], onLocationChange, {
  flush: "post",
});
onMounted(() => {
  onLocationChange();
  if (typeof ResizeObserver !== "undefined" && viewport.value && track.value) {
    observer = new ResizeObserver(onPathResize);
    observer.observe(viewport.value);
    observer.observe(track.value);
  } else {
    window.addEventListener("resize", onPathResize);
  }
});
onBeforeUnmount(() => {
  observer?.disconnect();
  window.removeEventListener("resize", onPathResize);
});

const items = computed(() => {
  const base = props.base.replace(/\/+$/, "");
  if (!route.path.startsWith(`${base}/`)) return [];
  let url = `${base}/`;
  return route.path
    .slice(url.length)
    .split("/")
    .filter(Boolean)
    .map((part) => {
      url += `${part}/`;
      let name = part;
      try {
        name = decodeURIComponent(part);
      } catch {
        // Keep malformed escapes visible without breaking the whole path.
      }
      return { name, url };
    });
});

const element = computed(() => {
  if (props.noLink) {
    return "span";
  }

  return "router-link";
});
</script>
