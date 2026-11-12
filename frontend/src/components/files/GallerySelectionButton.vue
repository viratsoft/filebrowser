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
    <i class="material-icons gallery-select-circle" aria-hidden="true">{{
      selected ? "check" : ""
    }}</i>
  </button>
</template>

<script setup lang="ts">
defineProps<{ selected: boolean; disabled: boolean }>();
const emit = defineEmits<{ (event: "toggle"): void }>();
</script>

<style scoped>
.gallery-select {
  -webkit-tap-highlight-color: transparent;
}

#previewer .gallery-select .gallery-select-circle {
  display: inline-grid;
  place-items: center;
  box-sizing: border-box;
  width: 1.4rem;
  height: 1.4rem;
  border: 2px solid currentColor;
  border-radius: 50%;
  background: rgb(0 0 0 / 45%);
  font-size: 1rem;
  line-height: 1;
  text-shadow: none;
}

#previewer .gallery-select[aria-pressed="true"] .gallery-select-circle {
  border-color: transparent;
  background: #1769c2;
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
</style>
