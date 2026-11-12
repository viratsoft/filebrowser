<template>
  <button
    type="button"
    class="action gallery-select"
    :disabled="disabled"
    :aria-pressed="selected"
    :aria-label="$t('buttons.selectGalleryFile')"
    :title="$t('buttons.selectGalleryFile')"
    @click="emit('toggle')"
  >
    <span class="gallery-select-circle" aria-hidden="true"></span>
  </button>
</template>

<script setup lang="ts">
defineProps<{ selected: boolean; disabled: boolean }>();
const emit = defineEmits<{ (event: "toggle"): void }>();
</script>

<style scoped>
.gallery-select {
  display: grid;
  place-items: center;
  width: 2.75rem;
  height: 2.75rem;
  -webkit-tap-highlight-color: transparent;
}

#previewer .gallery-select .gallery-select-circle {
  display: inline-grid;
  place-items: center;
  box-sizing: border-box;
  width: 1.4rem;
  height: 1.4rem;
  border: 2px solid #fff;
  border-radius: 50%;
  background: rgb(0 0 0 / 45%);
  transition:
    background-color 140ms ease-out,
    border-color 140ms ease-out,
    transform 180ms cubic-bezier(0.2, 0.8, 0.35, 1.3);
}

#previewer .gallery-select[aria-pressed="true"] .gallery-select-circle {
  border-color: transparent;
  background: #1769c2;
}

#previewer .gallery-select:enabled:active .gallery-select-circle {
  transform: scale(0.87);
}

#previewer .gallery-select .gallery-select-circle::after {
  content: "";
  box-sizing: border-box;
  width: 0.4rem;
  height: 0.68rem;
  border: solid #fff;
  border-width: 0 2px 2px 0;
  opacity: 0;
  transform: translateY(-0.06rem) rotate(45deg) scale(0.55);
  transition:
    opacity 100ms ease-out,
    transform 180ms cubic-bezier(0.2, 0.8, 0.35, 1.3);
}

#previewer .gallery-select[aria-pressed="true"] .gallery-select-circle::after {
  opacity: 1;
  transform: translateY(-0.06rem) rotate(45deg) scale(1);
}

.gallery-select:focus-visible {
  outline: 2px solid #fff;
  outline-offset: -3px;
}

@media (hover: none) {
  #previewer .gallery-select:hover {
    background-color: transparent;
  }
}

@media (prefers-reduced-motion: reduce) {
  #previewer .gallery-select,
  #previewer .gallery-select .gallery-select-circle,
  #previewer .gallery-select .gallery-select-circle::after {
    transition: none;
  }

  #previewer .gallery-select:enabled:active .gallery-select-circle {
    transform: none;
  }
}
</style>
