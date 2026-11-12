<template>
  <header-bar showMenu showLogo />
  <section
    v-if="file && !layoutStore.loading"
    class="card file-details"
    aria-labelledby="file-details-name"
  >
    <div class="card-title">
      <h2 id="file-details-name">{{ file.name }}</h2>
    </div>
    <div class="card-content">
      <dl>
        <dt>{{ $t("files.size") }}</dt>
        <dd>{{ filesize(file.size) }}</dd>
        <dt>{{ $t("files.lastModified") }}</dt>
        <dd>
          <time :datetime="file.modified">{{ modified }}</time>
        </dd>
      </dl>
    </div>
    <div class="card-action">
      <router-link
        class="button"
        :to="{ path: route.path, query: { ...route.query, view: undefined } }"
        replace
      >
        {{ $t("sidebar.preview") }}
      </router-link>
      <a
        v-if="authStore.user?.perm.download"
        class="button button--flat"
        :href="api.getDownloadURL(file, false)"
        download
      >
        {{ $t("buttons.download") }}
      </a>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import dayjs from "dayjs";
import { files as api } from "@/api";
import HeaderBar from "@/components/header/HeaderBar.vue";
import { useAuthStore } from "@/stores/auth";
import { useFileStore } from "@/stores/file";
import { useLayoutStore } from "@/stores/layout";
import { filesize } from "@/utils";

const route = useRoute();
const authStore = useAuthStore();
const fileStore = useFileStore();
const layoutStore = useLayoutStore();
const file = computed(() => fileStore.req);
const modified = computed(() => {
  const date = dayjs(file.value?.modified);
  return authStore.user?.dateFormat ? date.format("L LT") : date.fromNow();
});
</script>

<style scoped>
.file-details {
  width: min(100% - 2rem, 32rem);
  margin: 1rem auto;
}

.file-details h2 {
  overflow-wrap: anywhere;
}

.file-details dl {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 0.65rem 1rem;
  margin: 0;
}

.file-details dt {
  font-weight: 500;
}

.file-details dd {
  margin: 0;
  overflow-wrap: anywhere;
}

.file-details .card-action {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}
</style>
