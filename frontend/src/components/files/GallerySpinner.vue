<template>
  <span
    class="gallery-spinner"
    role="progressbar"
    :aria-label="label"
    aria-busy="true"
  >
    <svg viewBox="0 0 40 40" aria-hidden="true">
      <circle cx="20" cy="20" r="16" />
    </svg>
  </span>
</template>

<script setup lang="ts">
defineProps<{ label: string }>();
</script>

<style scoped>
.gallery-spinner {
  display: inline-flex;
  width: 32px;
  height: 32px;
  color: #fff;
  flex-shrink: 0;
}
.gallery-spinner svg {
  width: 100%;
  height: 100%;
  animation: gallery-ring-rotate 1.8s linear infinite;
}
.gallery-spinner circle {
  fill: none;
  stroke: currentColor;
  stroke-width: 3;
  stroke-linecap: round;
  stroke-dasharray: 1 100;
  transform-origin: center;
  animation: gallery-ring-dash 1.4s ease-in-out infinite;
}
@keyframes gallery-ring-rotate {
  to {
    transform: rotate(360deg);
  }
}
@keyframes gallery-ring-dash {
  0% {
    stroke-dasharray: 1 100;
    stroke-dashoffset: 0;
  }
  50% {
    stroke-dasharray: 70 100;
    stroke-dashoffset: -25;
  }
  100% {
    stroke-dasharray: 1 100;
    stroke-dashoffset: -100;
  }
}
@media (prefers-reduced-motion: reduce) {
  .gallery-spinner svg {
    animation-duration: 4s;
  }
  .gallery-spinner circle {
    animation: none;
    stroke-dasharray: 60 100;
  }
}
</style>
