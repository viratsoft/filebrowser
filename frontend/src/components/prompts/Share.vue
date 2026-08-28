<template>
  <div class="card floating" id="share">
    <div class="card-title">
      <h2>{{ $t("buttons.share") }}</h2>
    </div>

    <template v-if="listing">
      <div class="card-content">
        <table>
          <tr>
            <th>{{ $t("prompts.shareName") }}</th>
            <th>{{ $t("settings.shareDuration") }}</th>
            <th></th>
            <th></th>
            <th></th>
          </tr>

          <tr v-for="link in links" :key="link.hash">
            <td>{{ link.name || link.hash }}</td>
            <td>
              <template v-if="link.expire !== 0">{{
                humanTime(link.expire)
              }}</template>
              <template v-else>{{ $t("permanent") }}</template>
            </td>
            <td class="small">
              <button
                class="action"
                :aria-label="$t('buttons.copyToClipboard')"
                :title="$t('buttons.copyToClipboard')"
                @click="copyToClipboard(buildLink(link))"
              >
                <i class="material-icons">content_paste</i>
              </button>
            </td>
            <td class="small">
              <button
                class="action"
                :aria-label="$t('buttons.copyDownloadLinkToClipboard')"
                :title="$t('buttons.copyDownloadLinkToClipboard')"
                :disabled="!!link.hasPassword"
                @click="copyToClipboard(buildDownloadLink(link))"
              >
                <i class="material-icons">content_paste_go</i>
              </button>
            </td>
            <td class="small">
              <button
                class="action"
                @click="deleteLink($event, link)"
                :aria-label="$t('buttons.delete')"
                :title="$t('buttons.delete')"
              >
                <i class="material-icons">delete</i>
              </button>
            </td>
          </tr>
        </table>
      </div>

      <div class="card-action">
        <button
          class="button button--flat button--grey"
          @click="closeHovers"
          :aria-label="$t('buttons.close')"
          :title="$t('buttons.close')"
          tabindex="2"
        >
          {{ $t("buttons.close") }}
        </button>
        <button
          id="focus-prompt"
          class="button button--flat button--blue"
          @click="() => switchListing()"
          :aria-label="$t('buttons.new')"
          :title="$t('buttons.new')"
          tabindex="1"
        >
          {{ $t("buttons.new") }}
        </button>
      </div>
    </template>

    <template v-else>
      <div class="card-content">
        <p>{{ $t("settings.shareDuration") }}</p>
        <div class="input-group input">
          <vue-number-input
            center
            controls
            size="small"
            :max="2147483647"
            :min="0"
            @keyup.enter="submit"
            v-model="time"
            tabindex="1"
          />
          <select
            class="right"
            v-model="unit"
            :aria-label="$t('time.unit')"
            tabindex="2"
          >
            <option value="seconds">{{ $t("time.seconds") }}</option>
            <option value="minutes">{{ $t("time.minutes") }}</option>
            <option value="hours">{{ $t("time.hours") }}</option>
            <option value="days">{{ $t("time.days") }}</option>
          </select>
        </div>
        <p>{{ $t("prompts.optionalPassword") }}</p>
        <input
          class="input input--block"
          type="password"
          v-model.trim="password"
          tabindex="3"
        />
        <p class="share-field-label">{{ $t("prompts.shareName") }}</p>
        <input class="input input--block" v-model.trim="name" :placeholder="$t('prompts.shareNameHint')" />
        <label v-if="isDirectory" class="checkbox share-upload-option">
          <input type="checkbox" v-model="allowUpload" tabindex="4" />
          {{ $t("prompts.allowUpload") }}
        </label>
        <p v-if="isDirectory" class="share-hint">{{ $t("prompts.allowUploadHint") }}</p>
        <label v-if="isDirectory && allowUpload" class="checkbox share-upload-only-option">
          <input type="checkbox" v-model="uploadOnly" />
          {{ $t("prompts.uploadOnly") }}
        </label>
        <p v-if="isDirectory && allowUpload" class="share-hint">{{ $t("prompts.uploadOnlyHint") }}</p>
        <label v-if="isDirectory && uploadOnly" class="checkbox share-session-folder-option">
          <input type="checkbox" v-model="sessionUploadFolder" />
          {{ $t("prompts.sessionUploadFolder") }}
        </label>
        <p v-if="isDirectory && uploadOnly" class="share-hint">{{ $t("prompts.sessionUploadFolderHint") }}</p>
        <label v-if="isDirectory && allowUpload" class="checkbox share-owner-option">
          <input type="checkbox" v-model="matchFolderOwner" />
          {{ $t("prompts.matchFolderOwner") }}
        </label>
        <p v-if="isDirectory && allowUpload" class="share-hint">{{ $t("prompts.matchFolderOwnerHint") }}</p>
      </div>

      <div class="card-action">
        <button
          class="button button--flat button--grey"
          @click="() => switchListing()"
          :aria-label="$t('buttons.cancel')"
          :title="$t('buttons.cancel')"
          tabindex="5"
        >
          {{ $t("buttons.cancel") }}
        </button>
        <button
          id="focus-prompt"
          class="button button--flat button--blue"
          @click="submit"
          :aria-label="$t('buttons.share')"
          :title="$t('buttons.share')"
          tabindex="4"
        >
          {{ $t("buttons.share") }}
        </button>
      </div>
    </template>
  </div>
</template>

<script>
import { mapActions, mapState } from "pinia";
import { useFileStore } from "@/stores/file";
import * as api from "@/api/index";
import dayjs from "dayjs";
import { useLayoutStore } from "@/stores/layout";
import { copy } from "@/utils/clipboard";

export default {
  name: "share",
  data: function () {
    return {
      time: 0,
      unit: "hours",
      links: [],
      clip: null,
      password: "",
      allowUpload: false,
      uploadOnly: false,
      sessionUploadFolder: false,
      matchFolderOwner: false,
      name: "",
      listing: true,
    };
  },
  inject: ["$showError", "$showSuccess"],
  computed: {
    ...mapState(useFileStore, [
      "req",
      "selected",
      "selectedCount",
      "isListing",
    ]),
    url() {
      if (!this.isListing) {
        return this.$route.path;
      }

      if (this.selectedCount === 0 || this.selectedCount > 1) {
        // This shouldn't happen.
        return;
      }

      return this.req.items[this.selected[0]].url;
    },
    isDirectory() {
      if (!this.isListing) return this.req?.isDir;
      return this.selectedCount === 1 && this.req.items[this.selected[0]].isDir;
    },
  },
  watch: {
    allowUpload(enabled) {
      if (!enabled) {
        this.uploadOnly = false;
        this.sessionUploadFolder = false;
        this.matchFolderOwner = false;
      }
    },
    uploadOnly(enabled) {
      if (!enabled) this.sessionUploadFolder = false;
    },
  },
  async beforeMount() {
    try {
      const links = await api.share.get(this.url);
      this.links = links;
      this.sort();

      if (this.links.length == 0) {
        this.listing = false;
      }
    } catch (e) {
      this.$showError(e);
    }
  },
  methods: {
    ...mapActions(useLayoutStore, ["closeHovers"]),
    copyToClipboard: function (text) {
      copy({ text }).then(
        () => {
          // clipboard successfully set
          this.$showSuccess(this.$t("success.linkCopied"));
        },
        () => {
          // clipboard write failed
          copy({ text }, { permission: true }).then(
            () => {
              // clipboard successfully set
              this.$showSuccess(this.$t("success.linkCopied"));
            },
            (e) => {
              // clipboard write failed
              this.$showError(e);
            }
          );
        }
      );
    },
    submit: async function () {
      try {
        let res = null;

        if (!this.time) {
          res = await api.share.create(
            this.url,
            this.password,
            "",
            "hours",
            this.allowUpload,
            this.uploadOnly,
            this.sessionUploadFolder,
            this.matchFolderOwner,
            this.name
          );
        } else {
          res = await api.share.create(
            this.url,
            this.password,
            this.time,
            this.unit,
            this.allowUpload,
            this.uploadOnly,
            this.sessionUploadFolder,
            this.matchFolderOwner,
            this.name
          );
        }

        this.links.push(res);
        this.sort();

        this.time = 0;
        this.unit = "hours";
        this.password = "";
        this.allowUpload = false;
        this.uploadOnly = false;
        this.sessionUploadFolder = false;
        this.matchFolderOwner = false;
        this.name = "";

        this.listing = true;
      } catch (e) {
        this.$showError(e);
      }
    },
    deleteLink: async function (event, link) {
      event.preventDefault();
      try {
        await api.share.remove(link.hash);
        this.links = this.links.filter((item) => item.hash !== link.hash);

        if (this.links.length == 0) {
          this.listing = false;
        }
      } catch (e) {
        this.$showError(e);
      }
    },
    humanTime(time) {
      return dayjs(time * 1000).fromNow();
    },
    buildLink(share) {
      return api.share.getShareURL(share);
    },
    buildDownloadLink(share) {
      return api.pub.getDownloadURL(
        {
          hash: share.hash,
          path: "",
        },
        true
      );
    },
    sort() {
      this.links = this.links.sort((a, b) => {
        if (a.expire === 0) return -1;
        if (b.expire === 0) return 1;
        return new Date(a.expire) - new Date(b.expire);
      });
    },
    switchListing() {
      if (this.links.length == 0 && !this.listing) {
        this.closeHovers();
      }

      this.listing = !this.listing;
    },
  },
};
</script>

<style scoped>
.share-upload-option {
  display: block;
  margin-top: 1.25rem;
}
.share-upload-only-option { display: block; margin-top: 1rem; }
.share-session-folder-option { display: block; margin-top: 1rem; }
.share-owner-option { display: block; margin-top: 1rem; }
.share-hint { color: var(--text-secondary, #777); font-size: .85rem; margin: .35rem 0 0 2rem; }
.share-field-label { margin-top: 1.25rem; }
</style>
