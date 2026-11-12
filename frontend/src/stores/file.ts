import { defineStore } from "pinia";
import { removeLastDir } from "@/utils/url";

export const useFileStore = defineStore("file", {
  // convert to a function
  state: (): {
    req: Resource | null;
    oldReq: Resource | null;
    reload: boolean;
    selected: number[];
    multiple: boolean;
    isFiles: boolean;
    preselect: string | null;
    gallerySelection: { folder: string; paths: string[] } | null;
  } => ({
    req: null,
    oldReq: null,
    reload: false,
    selected: [],
    multiple: false,
    isFiles: false,
    preselect: null,
    gallerySelection: null,
  }),
  getters: {
    selectedCount: (state) => state.selected.length,
    allItemsSelected: (state) => {
      const items = state.req?.isDir ? state.req.items : [];
      if (!items?.length) return false;
      const selected = new Set(state.selected);
      return (
        selected.size === items.length &&
        items.every((_, index) => selected.has(index))
      );
    },
    // route: () => {
    //   const routerStore = useRouterStore();
    //   return routerStore.router.currentRoute;
    // },
    // isFiles: (state) => {
    //   const layoutStore = useLayoutStore();
    //   return !layoutStore.loading && state.route._value.name === "Files";
    // },
    isListing: (state) => {
      return state.isFiles && state?.req?.isDir;
    },
  },
  actions: {
    toggleGallerySelection(routePath: string, filePath: string) {
      const folder = removeLastDir(routePath);
      const paths =
        this.gallerySelection?.folder === folder
          ? this.gallerySelection.paths
          : [];
      this.gallerySelection = {
        folder,
        paths: paths.includes(filePath)
          ? paths.filter((path) => path !== filePath)
          : [...paths, filePath],
      };
    },
    restoreGallerySelection(routePath: string) {
      const selection = this.gallerySelection;
      const resource = this.req;
      if (!selection || !resource) return false;
      if (!resource.isDir) {
        if (removeLastDir(routePath) !== selection.folder) {
          this.gallerySelection = null;
        }
        return false;
      }
      this.gallerySelection = null;
      if (
        resource.uploadOnly ||
        routePath.replace(/\/+$/, "") !== selection.folder
      ) {
        return false;
      }
      const paths = new Set(selection.paths);
      this.selected = resource.items
        .filter((item) => !item.isDir && paths.has(item.path))
        .map((item) => item.index);
      this.multiple = this.selected.length > 0;
      this.preselect = null;
      return true;
    },
    // no context as first argument, use `this` instead
    toggleMultiple() {
      this.multiple = !this.multiple;
    },
    toggleAllSelection(publicShare = false) {
      const publicListing =
        publicShare && this.req?.isDir && !this.req.uploadOnly;
      if (
        (!this.isListing && !publicListing) ||
        this.req?.uploadOnly ||
        !this.req?.items?.length
      )
        return;
      this.multiple = true;
      this.selected = this.allItemsSelected
        ? []
        : this.req.items.map((_, index) => index);
    },
    updateRequest(value: Resource | null) {
      const selectedItems = this.selected.map((i) => this.req?.items?.[i]);
      this.oldReq = this.req;
      this.req = value;

      this.selected = [];

      if (!this.req?.items) return;
      this.selected = this.req.items
        .filter((item) =>
          selectedItems.some((rItem) => rItem?.url === item.url)
        )
        .map((item) => item.index);
    },
    removeSelected(value: any) {
      const i = this.selected.indexOf(value);
      if (i === -1) return;
      this.selected.splice(i, 1);
    },
    // easily reset state using `$reset`
    clearFile() {
      this.$reset();
    },
  },
});
