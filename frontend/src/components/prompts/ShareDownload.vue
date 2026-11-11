<template>
  <div
    class="card floating share-download"
    role="dialog"
    aria-labelledby="share-download-title"
  >
    <div class="card-title">
      <h2 id="share-download-title">{{ t("prompts.download") }}</h2>
    </div>

    <div class="card-content">
      <p v-if="fileCount !== undefined || selectedSize" class="download-total">
        <span v-if="fileCount !== undefined">
          <strong>{{ t(fileCountLabel) }}:</strong> {{ n(fileCount) }}
        </span>
        <span v-if="selectedSize">
          <strong
            >{{
              t(
                includesFolders
                  ? "prompts.shareDownloadSelectedSize"
                  : "prompts.shareDownloadTotalSize"
              )
            }}:</strong
          >
          {{ selectedSize }}
        </span>
      </p>
      <p class="download-hint">
        {{
          t(
            includesFolders
              ? "prompts.shareDownloadFolders"
              : "prompts.shareDownloadHint"
          )
        }}
      </p>
      <button
        :id="includesFolders ? undefined : 'focus-prompt'"
        type="button"
        class="button button--block download-option"
        :disabled="busy !== null || includesFolders"
        :aria-busy="busy === 'all'"
        @click="start('all')"
      >
        <gallery-spinner v-if="busy === 'all'" :label="t('files.loading')" />
        <i v-else class="material-icons" aria-hidden="true">file_download</i>
        {{ t("prompts.shareDownloadAll") }}
      </button>
      <button
        :id="includesFolders ? 'focus-prompt' : undefined"
        type="button"
        class="button button--block button--flat download-option zip-option"
        :disabled="busy !== null"
        :aria-busy="busy === 'zip'"
        @click="start('zip')"
      >
        <gallery-spinner v-if="busy === 'zip'" :label="t('files.loading')" />
        <i v-else class="material-icons" aria-hidden="true">folder_zip</i>
        {{ t("prompts.shareDownloadZip") }}
      </button>
      <p v-if="error" class="download-error" role="alert">{{ error }}</p>
    </div>

    <div class="card-action">
      <button
        type="button"
        class="button button--flat"
        @click="layoutStore.closeHovers()"
      >
        {{ t("buttons.cancel") }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onDeactivated, ref } from "vue";
import { useI18n } from "vue-i18n";
import { filesize } from "filesize";
import { useLayoutStore } from "@/stores/layout";
import { StatusError } from "@/api/utils";
import { ShareFolderDownloadError } from "@/api/pub";
import GallerySpinner from "@/components/files/GallerySpinner.vue";

const { t, n } = useI18n();
const layoutStore = useLayoutStore();
const includesFolders = computed(
  () => !!layoutStore.currentPrompt?.props?.includesFolders
);
const fileCount = computed<number | undefined>(
  () => layoutStore.currentPrompt?.props?.fileCount
);
const fileCountLabel = computed(() => {
  const scope = layoutStore.currentPrompt?.props?.fileCountScope;
  if (scope === "selected") return "prompts.shareDownloadSelectedFiles";
  if (scope === "current-folder") return "prompts.shareDownloadCurrentFiles";
  return "prompts.totalFiles";
});
const selectedSize = computed(() => {
  const bytes = layoutStore.currentPrompt?.props?.selectedBytes;
  return typeof bytes === "number" && Number.isFinite(bytes) && bytes >= 0
    ? filesize(bytes, { base: 10, symbols: { kB: "KB" } })
    : "";
});
const busy = ref<"all" | "zip" | null>(null);
const error = ref("");
let active: AbortController | null = null;

function cancel() {
  active?.abort();
  active = null;
  busy.value = null;
  error.value = "";
}

async function start(mode: "all" | "zip") {
  const prompt = layoutStore.currentPrompt;
  if (
    !prompt?.confirm ||
    busy.value ||
    (mode === "all" && includesFolders.value)
  )
    return;

  const controller = new AbortController();
  active = controller;
  busy.value = mode;
  error.value = "";
  try {
    await prompt.confirm(mode, controller.signal);
    if (!controller.signal.aborted && layoutStore.currentPrompt === prompt) {
      layoutStore.closeHovers();
    }
  } catch (failure) {
    if (!controller.signal.aborted) {
      const status =
        failure instanceof StatusError ? failure.status : undefined;
      error.value = t(
        failure instanceof ShareFolderDownloadError
          ? failure.reason === "empty"
            ? "prompts.shareDownloadEmpty"
            : "prompts.shareDownloadLinkedFolder"
          : status === 401 || status === 403
            ? "prompts.shareDownloadAccessError"
            : status === 404
              ? "prompts.shareDownloadMissing"
              : "prompts.shareDownloadError"
      );
    }
  } finally {
    controller.abort();
    if (active === controller) {
      active = null;
      busy.value = null;
    }
  }
}

onBeforeUnmount(cancel);
onDeactivated(cancel);
</script>

<style scoped>
.share-download {
  width: min(23rem, calc(100vw - 2rem));
  max-height: min(95%, calc(100dvh - 2rem));
  overflow-y: auto;
  border-radius: 0.7rem;
}
.share-download .card-title {
  padding: 1.15rem 1.15rem 0.55rem;
}
.share-download .card-content {
  padding: 0.65rem 1.15rem 0.35rem;
}
.share-download .card-action {
  padding: 0.35rem 1.15rem 0.9rem;
}
.share-download .card-action .button {
  min-height: 2.5rem;
  border-radius: 0.45rem;
}
.download-total {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem 1rem;
  margin: 0 0 0.65rem;
  font-size: 1rem;
}
.download-hint {
  margin: 0 0 0.9rem;
  color: var(--textSecondary);
}
.download-option {
  display: flex;
  min-height: 2.75rem;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
  padding: 0.45rem 0.85rem;
  border-radius: 0.5rem;
  font-size: 0.95rem;
  font-weight: 500;
  text-transform: none;
}
.zip-option {
  border: 1px solid var(--borderPrimary);
}
.share-download .button:focus-visible {
  outline: 2px solid var(--blue);
  outline-offset: 2px;
}
.download-option .material-icons,
.download-option :deep(.gallery-spinner) {
  font-size: 1.15rem;
  width: 1.15rem;
  height: 1.15rem;
  color: inherit;
}
.download-error {
  margin: 0.8rem 0 0;
  color: var(--red);
}
</style>
