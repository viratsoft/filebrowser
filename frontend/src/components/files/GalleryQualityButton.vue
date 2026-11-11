<template>
  <button
    type="button"
    class="action"
    :disabled="disabled"
    :aria-pressed="original"
    :title="label"
    :aria-label="label"
    @click="emit('toggle')"
  >
    <i class="material-icons" aria-hidden="true">{{
      original ? "photo_size_select_large" : "hd"
    }}</i>
  </button>
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
