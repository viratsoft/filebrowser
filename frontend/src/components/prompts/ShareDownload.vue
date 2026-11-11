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
      <div
        v-if="calculateTotals"
        class="download-folder-details"
        :aria-busy="counting"
      >
        <strong>{{ t("prompts.folderDetails") }}:</strong>
        <button
          type="button"
          class="button button--flat calculate-details"
          :class="{ 'calculate-details-icon': counting || calculatedTotals }"
          :disabled="counting || busy !== null"
          :aria-label="
            t(
              counting
                ? 'prompts.calculatingSize'
                : calculatedTotals
                  ? 'prompts.refreshFolderDetails'
                  : 'prompts.calculateFolderDetails'
            )
          "
          :title="
            calculatedTotals && !counting
              ? t('prompts.refreshFolderDetails')
              : undefined
          "
          @click="loadCount"
        >
          <gallery-spinner
            v-if="counting"
            :label="t('prompts.calculatingSize')"
          />
          <i
            v-else-if="calculatedTotals"
            class="material-icons"
            aria-hidden="true"
            >refresh</i
          >
          <template v-else>{{ t("prompts.calculateFolderDetails") }}</template>
        </button>
      </div>
      <p
        v-if="fileCount !== undefined || selectedSize"
        class="download-total"
        aria-live="polite"
      >
        <span v-if="fileCount !== undefined">
          <strong>{{ t("prompts.totalFiles") }}:</strong> {{ n(fileCount) }}
        </span>
        <span v-if="selectedSize">
          <strong>{{ t("prompts.shareDownloadTotalSize") }}:</strong>
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
        :disabled="
          busy !== null || includesFolders || counting || fileCount === 0
        "
        :aria-busy="busy === 'all'"
        @click="start('all')"
      >
        <gallery-spinner v-if="busy === 'all'" :label="t('files.loading')" />
        <i v-else class="material-icons" aria-hidden="true">file_download</i>
        {{ t("prompts.shareDownloadAll") }}
      </button>
      <a
        :id="includesFolders ? 'focus-prompt' : undefined"
        :href="archiveURL"
        class="button button--block button--flat download-option zip-option"
        :aria-disabled="busy !== null || counting"
        :aria-busy="busy === 'zip'"
        :tabindex="busy === null && !counting ? undefined : -1"
        referrerpolicy="no-referrer"
        @click.prevent="start('zip')"
      >
        <gallery-spinner v-if="busy === 'zip'" :label="t('files.loading')" />
        <i v-else class="material-icons" aria-hidden="true">folder_zip</i>
        {{ t("prompts.shareDownloadZip") }}
      </a>
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
import { computed, onBeforeUnmount, onDeactivated, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { filesize } from "filesize";
import { useLayoutStore } from "@/stores/layout";
import { StatusError } from "@/api/utils";
import GallerySpinner from "@/components/files/GallerySpinner.vue";

const { t, n } = useI18n();
const layoutStore = useLayoutStore();
const includesFolders = computed(
  () => !!layoutStore.currentPrompt?.props?.includesFolders
);
const archiveURL = computed<string | undefined>(
  () => layoutStore.currentPrompt?.props?.archiveURL
);
const calculateTotals = computed<
  | ((signal: AbortSignal) => Promise<{ fileCount: number; size: number }>)
  | undefined
>(() => layoutStore.currentPrompt?.props?.calculateTotals);
const calculatedTotals = ref<{ fileCount: number; size: number }>();
const counting = ref(false);
const fileCount = computed<number | undefined>(
  () =>
    calculatedTotals.value?.fileCount ??
    layoutStore.currentPrompt?.props?.fileCount
);
const selectedSize = computed(() => {
  const bytes =
    calculatedTotals.value?.size ??
    layoutStore.currentPrompt?.props?.selectedBytes;
  return typeof bytes === "number" && Number.isFinite(bytes) && bytes >= 0
    ? filesize(bytes, { base: 10, symbols: { kB: "KB" } })
    : "";
});
const busy = ref<"all" | "zip" | null>(null);
const error = ref("");
let active: AbortController | null = null;

async function loadCount() {
  const calculate = calculateTotals.value;
  if (!calculate || active) return;
  const controller = new AbortController();
  active = controller;
  counting.value = true;
  error.value = "";
  try {
    const totals = await calculate(controller.signal);
    if (active === controller) calculatedTotals.value = totals;
  } catch {
    if (active === controller) error.value = t("prompts.shareDownloadError");
  } finally {
    if (active === controller) {
      active = null;
      counting.value = false;
    }
  }
}

function cancel() {
  active?.abort();
  active = null;
  busy.value = null;
  counting.value = false;
  calculatedTotals.value = undefined;
  error.value = "";
}

async function start(mode: "all" | "zip") {
  const prompt = layoutStore.currentPrompt;
  if (
    !prompt?.confirm ||
    busy.value ||
    counting.value ||
    (mode === "all" && (includesFolders.value || fileCount.value === 0))
  )
    return;

  const controller = new AbortController();
  active = controller;
  busy.value = mode;
  error.value = "";
  try {
    const started = await prompt.confirm(mode, controller.signal);
    if (!controller.signal.aborted && layoutStore.currentPrompt === prompt) {
      if (started === false) {
        error.value = t("prompts.shareDownloadEmptyFolder");
      } else {
        layoutStore.closeHovers();
      }
    }
  } catch (failure) {
    if (!controller.signal.aborted) {
      const status =
        failure instanceof StatusError ? failure.status : undefined;
      error.value = t(
        status === 401 || status === 403
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

watch(() => layoutStore.currentPrompt, cancel);
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
.download-folder-details {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.2rem 0.5rem;
  margin-bottom: 0.45rem;
}
.calculate-details {
  min-height: 2.5rem;
  padding: 0.25rem 0.55rem;
  border-radius: 0.45rem;
  font-size: 0.9rem;
  text-transform: none;
}
.calculate-details-icon {
  display: inline-grid;
  place-items: center;
  width: 2.5rem;
  padding: 0;
}
.calculate-details-icon .material-icons,
.calculate-details-icon :deep(.gallery-spinner) {
  width: 1.15rem;
  height: 1.15rem;
  font-size: 1.15rem;
  color: inherit;
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
  box-sizing: border-box;
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
  line-height: normal;
  text-align: center;
  text-transform: none;
}
.zip-option {
  border: 1px solid var(--borderPrimary);
}
.zip-option,
.zip-option:hover,
.zip-option:visited {
  color: var(--dark-blue);
  text-decoration: none;
}
.zip-option[aria-disabled="true"] {
  opacity: 0.5;
  cursor: not-allowed;
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
