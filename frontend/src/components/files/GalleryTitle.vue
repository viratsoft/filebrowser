<template>
  <div class="gallery-title">
    <span class="gallery-filename" :title="filename">{{ filename }}</span>
    <span v-if="position > 0 && position <= total" class="gallery-count">
      {{ position }}/{{ total }}
    </span>
  </div>
</template>

<script setup lang="ts">
defineProps<{ filename: string; position: number; total: number }>();
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

.gallery-count {
  color: rgba(255, 255, 255, 0.75);
  font-size: 0.68rem;
  line-height: 1.15;
  font-variant-numeric: tabular-nums;
}
</style>
