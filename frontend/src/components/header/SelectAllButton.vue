<template>
  <button
    v-if="fileStore.isListing && fileStore.req?.items?.length"
    type="button"
    class="action select-all-action"
    :disabled="layoutStore.loading"
    :aria-label="label"
    :title="label"
    :aria-pressed="fileStore.allItemsSelected"
    @click="toggleSelection"
  >
    <i class="material-icons" aria-hidden="true">{{
      fileStore.allItemsSelected ? "deselect" : "select_all"
    }}</i>
  </button>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { useFileStore } from "@/stores/file";
import { useLayoutStore } from "@/stores/layout";

const fileStore = useFileStore();
const layoutStore = useLayoutStore();
const { t } = useI18n();
const label = computed(() =>
  t(fileStore.allItemsSelected ? "buttons.deselectAll" : "buttons.selectAll")
);

function toggleSelection() {
  if (layoutStore.loading) return;
  fileStore.toggleAllSelection();
}
</script>

<style scoped>
.select-all-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 3em;
  min-width: 3em;
  min-height: 3em;
  padding: 0;
}

.select-all-action[aria-pressed="true"] {
  color: var(--blue);
}

.select-all-action:focus-visible {
  outline: 2px solid var(--blue);
  outline-offset: -2px;
}
</style>
