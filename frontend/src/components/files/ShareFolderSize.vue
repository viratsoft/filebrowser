<template>
  <div class="share-folder-size" :aria-busy="loading">
    <div class="size-controls">
      <span v-if="details" aria-live="polite">
        <strong
          >{{
            t(details.incomplete ? "prompts.partialSize" : "prompts.size")
          }}:</strong
        >
        {{ filesize(details.size) }}
      </span>
      <button
        v-if="!details && !loading"
        type="button"
        class="button button--flat calculate-size"
        @click="calculate"
      >
        {{ t("prompts.calculateShareSize") }}
      </button>
      <button
        v-else
        type="button"
        class="button button--flat refresh-size"
        :disabled="loading"
        :aria-label="
          t(
            loading ? 'prompts.calculatingSize' : 'prompts.refreshFolderDetails'
          )
        "
        :title="
          t(
            loading ? 'prompts.calculatingSize' : 'prompts.refreshFolderDetails'
          )
        "
        @click="calculate"
      >
        <gallery-spinner v-if="loading" :label="t('prompts.calculatingSize')" />
        <i v-else class="material-icons" aria-hidden="true">refresh</i>
      </button>
    </div>
    <p v-if="error" class="size-note" role="alert">{{ error }}</p>
    <p v-else-if="details?.incomplete" class="size-note" role="status">
      {{ t("prompts.folderDetailsIncomplete") }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { pub as api } from "@/api";
import { StatusError } from "@/api/utils";
import { filesize } from "@/utils";
import GallerySpinner from "./GallerySpinner.vue";

const props = defineProps<{ url: string; password: string; token: string }>();
const { t } = useI18n();
const details = ref<{ size: number; incomplete?: boolean } | null>(null);
const loading = ref(false);
const error = ref("");
let active: AbortController | null = null;

function reset() {
  active?.abort();
  active = null;
  loading.value = false;
  details.value = null;
  error.value = "";
}

async function calculate() {
  if (active) return;
  const controller = new AbortController();
  active = controller;
  loading.value = true;
  error.value = "";
  try {
    const result = await api.folderSize(
      props.url,
      props.password,
      props.token,
      controller.signal
    );
    if (active === controller) details.value = result;
  } catch (failure) {
    if (active === controller) {
      const status =
        failure instanceof StatusError ? failure.status : undefined;
      if (status === 401 || status === 403 || status === 404)
        details.value = null;
      error.value = t(
        status === 503 ? "prompts.shareSizeBusy" : "prompts.shareSizeError"
      );
    }
  } finally {
    if (active === controller) {
      active = null;
      loading.value = false;
    }
  }
}

watch(() => [props.url, props.password, props.token], reset);
onBeforeUnmount(reset);
</script>

<style scoped>
.share-folder-size {
  min-width: 0;
  margin-inline-start: auto;
  text-align: end;
}

.size-controls {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.25rem;
  min-height: 2.5rem;
  white-space: nowrap;
}

.size-controls .button {
  margin: 0;
  min-height: 2.5rem;
}

.calculate-size {
  font-size: 0.9rem;
  text-transform: none;
}

.size-controls .refresh-size {
  display: inline-grid;
  place-items: center;
  width: 2.5rem;
  padding: 0;
}

.refresh-size :deep(.gallery-spinner) {
  width: 1.2rem;
  height: 1.2rem;
  color: var(--blue);
}

.refresh-size .material-icons {
  margin: 0;
  font-size: 1.15rem;
}

.size-note {
  margin: 0 0 0.35rem;
  color: var(--textSecondary);
  font-size: 0.8rem;
}
</style>
