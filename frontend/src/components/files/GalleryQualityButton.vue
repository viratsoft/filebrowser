<template>
  <div class="gallery-quality-footer">
    <button
      type="button"
      class="gallery-quality-toggle"
      role="switch"
      :disabled="disabled"
      :aria-checked="original"
      :title="label"
      :aria-label="t('buttons.hdQuality')"
      @click.stop="emit('toggle')"
    >
      <span aria-hidden="true">HD</span>
      <span class="gallery-quality-track" aria-hidden="true"></span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";

const props = defineProps<{ original: boolean; disabled: boolean }>();
const emit = defineEmits<{ (event: "toggle"): void }>();
const { t } = useI18n();
const label = computed(() =>
  t(props.original ? "buttons.previewQuality" : "buttons.originalQuality")
);
</script>

<style scoped>
.gallery-quality-footer {
  position: absolute;
  z-index: 3;
  inset-inline-end: max(1rem, env(safe-area-inset-right, 0px));
  bottom: calc(1rem + env(safe-area-inset-bottom, 0px));
}

html[dir="rtl"] .gallery-quality-footer {
  inset-inline-end: max(1rem, env(safe-area-inset-left, 0px));
}

.gallery-quality-toggle {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  min-height: 2.75rem;
  padding: 0 0.25rem;
  border: 0;
  border-radius: 0.35rem;
  background: transparent;
  color: #fff;
  font: inherit;
  font-size: 0.75rem;
  font-weight: 600;
  text-shadow:
    0 1px 3px #000,
    0 0 4px #000;
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}

.gallery-quality-track {
  position: relative;
  width: 1.875rem;
  height: 1.125rem;
  border-radius: 999px;
  background: #62666c;
  transition: background-color 180ms ease-out;
}

.gallery-quality-track::after {
  content: "";
  position: absolute;
  top: 0.125rem;
  inset-inline-start: 0.125rem;
  width: 0.875rem;
  height: 0.875rem;
  border-radius: 50%;
  background: #fff;
  transition:
    inset-inline-start 210ms cubic-bezier(0.2, 0.8, 0.35, 1.15),
    transform 150ms ease-out;
}

.gallery-quality-toggle:enabled:active .gallery-quality-track::after {
  transform: scale(0.85);
}

.gallery-quality-toggle[aria-checked="true"] .gallery-quality-track {
  background: #1769c2;
}

.gallery-quality-toggle[aria-checked="true"] .gallery-quality-track::after {
  inset-inline-start: calc(100% - 1rem);
}

.gallery-quality-toggle:focus-visible {
  outline: 2px solid #fff;
  outline-offset: 2px;
}

.gallery-quality-toggle:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

@media (prefers-reduced-motion: reduce) {
  .gallery-quality-track,
  .gallery-quality-track::after {
    transition: none;
  }

  .gallery-quality-toggle:enabled:active .gallery-quality-track::after {
    transform: none;
  }
}
</style>
