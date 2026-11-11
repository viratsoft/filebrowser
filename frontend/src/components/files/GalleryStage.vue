<template>
  <div
    class="gallery-stage"
    :class="{ 'is-changing': changing, 'going-back': backward }"
  >
    <slot />
  </div>
  <div v-if="changing" class="gallery-slide-loading">
    <gallery-spinner :label="t('files.loading')" />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import GallerySpinner from "./GallerySpinner.vue";

defineProps<{ changing: boolean; backward: boolean }>();
const { t } = useI18n();
</script>

<style scoped>
.gallery-stage {
  width: 100%;
  height: 100%;
  transition:
    opacity 180ms ease-out,
    transform 180ms cubic-bezier(0.2, 0.72, 0.22, 1);
}
.gallery-stage.is-changing {
  opacity: 0;
  transform: translate3d(-22px, 0, 0);
  pointer-events: none;
}
.gallery-stage.is-changing.going-back {
  transform: translate3d(22px, 0, 0);
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
}
</style>
