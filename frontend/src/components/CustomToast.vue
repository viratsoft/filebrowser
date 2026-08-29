<template>
  <div class="t-container">
    <span class="message" :title="message">{{ message }}</span>
    <button v-if="isReport" class="action" @click.stop="clicked">
      {{ reportText }}
    </button>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  message: string;
  reportText?: string;
  isReport?: boolean;
}>();

const clicked = () => {
  window.open("https://github.com/filebrowser/filebrowser/issues/new/choose");
};
</script>

<style scoped>
.t-container {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 100%;
  min-width: 0;
  padding: 5px 0;
  width: 100%;
}
.message {
  flex: 1 1 auto;
  font-size: 16px;
  line-height: 20px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.action {
  text-align: center;
  height: 40px;
  padding: 0 10px;
  margin-left: 20px;
  border-radius: 5px;
  color: white;
  cursor: pointer;
  border: thin solid currentColor;
}

html[dir="rtl"] .action {
  margin-left: initial;
  margin-right: 20px;
}

:global(.Vue-Toastification__toast.compact-conflict-toast) {
  border-radius: 8px;
  box-sizing: border-box;
  height: 64px;
  margin-bottom: 12px;
  max-width: min(580px, calc(100vw - 24px));
  min-height: 64px;
  min-width: 0;
  padding: 0 14px;
  width: min(580px, calc(100vw - 24px));
}

:global(.compact-conflict-toast .Vue-Toastification__toast-body),
:global(.compact-conflict-toast .Vue-Toastification__toast-component-body) {
  margin: 0;
  min-width: 0;
  padding: 0;
  width: 100%;
}

:global(.compact-conflict-toast) .message {
  color: #fff !important;
}

:global(.compact-conflict-toast .Vue-Toastification__icon) {
  flex: 0 0 28px;
  margin: 0 12px 0 0;
  width: 28px;
}

:global(.compact-conflict-toast .Vue-Toastification__close-button) {
  align-self: center;
  flex: 0 0 24px;
  font-size: 26px;
  line-height: 24px;
  margin: 0 0 0 12px;
  opacity: 0.9;
  padding: 0;
}
</style>
