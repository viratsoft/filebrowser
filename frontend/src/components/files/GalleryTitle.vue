<template>
  <div class="gallery-title">
    <span class="gallery-filename" :title="filename">{{ filename }}</span>
    <div v-if="hasPosition || sizeLabel" class="gallery-details">
      <span v-if="hasPosition">{{ position }}/{{ total }}</span>
      <span v-if="hasPosition && sizeLabel" aria-hidden="true">·</span>
      <span v-if="sizeLabel">{{ sizeLabel }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { filesize } from "filesize";

const props = defineProps<{
  filename: string;
  position: number;
  total: number;
  bytes?: number;
}>();

const hasPosition = computed(
  () => props.position > 0 && props.position <= props.total
);
const sizeLabel = computed(() => {
  const bytes = props.bytes;
  return bytes !== undefined && Number.isFinite(bytes) && bytes >= 0
    ? filesize(bytes, { base: 10, symbols: { kB: "KB" } })
    : "";
});
</script>

<style scoped>
.gallery-title {
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
  flex-direction: column;
  justify-content: center;
  gap: 0.15rem;
  padding-inline: 1rem;
  text-shadow: 1px 1px 1px #000;
}

.gallery-filename {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.95rem;
  line-height: 1.2;
}

.gallery-details {
  display: flex;
  align-items: baseline;
  gap: 0.35rem;
  overflow: hidden;
  white-space: nowrap;
  color: rgba(255, 255, 255, 0.75);
  font-size: 0.68rem;
  line-height: 1.15;
  font-variant-numeric: tabular-nums;
}
</style>
