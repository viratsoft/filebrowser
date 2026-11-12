import { beforeEach, describe, expect, it } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { useFileStore } from "../file";

function listing(count: number) {
  const store = useFileStore();
  store.isFiles = true;
  store.req = {
    isDir: true,
    items: Array.from({ length: count }, (_, index) => ({
      index,
      name: `item-${index}`,
      path: `/current/item-${index}`,
      url: `/files/current/item-${index}`,
      isDir: index % 2 === 0,
    })),
  } as Resource;
  return store;
}

describe("select all in the current listing", () => {
  beforeEach(() => setActivePinia(createPinia()));

  it("selects both files and folders, including entries beyond the render limit", () => {
    const store = listing(155);
    store.toggleAllSelection();
    expect(store.selected).toEqual(Array.from({ length: 155 }, (_, i) => i));
    expect(store.selectedCount).toBe(155);
    expect(store.multiple).toBe(true);
    expect(store.allItemsSelected).toBe(true);
  });

  it("clears all on the next click without leaving multiple-selection mode", () => {
    const store = listing(3);
    store.toggleAllSelection();
    store.toggleAllSelection();
    expect(store.selected).toEqual([]);
    expect(store.allItemsSelected).toBe(false);
    expect(store.multiple).toBe(true);
  });

  it("completes a partial selection without duplicates or stale indices", () => {
    const store = listing(4);
    store.selected = [1, 1, 999];
    store.toggleAllSelection();
    expect(store.selected).toEqual([0, 1, 2, 3]);
  });

  it("recomputes all-selected when one item is deselected manually", () => {
    const store = listing(3);
    store.toggleAllSelection();
    store.removeSelected(1);
    expect(store.allItemsSelected).toBe(false);
    store.toggleAllSelection();
    expect(store.selected).toEqual([0, 1, 2]);
  });

  it("does nothing for an empty, loading-without-data, or non-directory view", () => {
    const store = listing(0);
    store.toggleAllSelection();
    expect(store.multiple).toBe(false);
    store.req = null;
    store.toggleAllSelection();
    expect(store.selected).toEqual([]);
    store.req = { isDir: false, items: [] } as unknown as Resource;
    store.toggleAllSelection();
    expect(store.multiple).toBe(false);
  });

  it("cannot change the selection on public share routes", () => {
    const store = listing(3);
    store.isFiles = false;
    store.toggleAllSelection();
    expect(store.selected).toEqual([]);
    expect(store.multiple).toBe(false);
  });

  it("allows an explicit readable-share selection including offscreen items, but never upload-only", () => {
    const store = listing(155);
    store.isFiles = false;
    store.toggleAllSelection(true);
    expect(store.selected).toHaveLength(155);
    store.toggleAllSelection(true);
    expect(store.selected).toEqual([]);
    store.req!.uploadOnly = true;
    store.toggleAllSelection(true);
    expect(store.selected).toEqual([]);
  });

  it("handles a single item and resets normally when leaving the listing", () => {
    const store = listing(1);
    store.toggleAllSelection();
    expect(store.selected).toEqual([0]);
    store.clearFile();
    expect(store.allItemsSelected).toBe(false);
    expect(store.selected).toEqual([]);
    expect(store.multiple).toBe(false);
  });
});

describe("restore slideshow selection", () => {
  beforeEach(() => setActivePinia(createPinia()));

  it.each(["/files/current", "/share/example/current"])(
    "restores only checked files at their current listing indices for %s",
    (folder) => {
      const store = listing(4);
      const directory = store.req!;
      store.toggleGallerySelection(`${folder}/item-1`, "/current/item-1");
      store.toggleGallerySelection(`${folder}/item-3`, "/current/item-3");
      store.toggleGallerySelection(`${folder}/item-3`, "/current/item-3");
      store.toggleGallerySelection(
        `${folder}/missing.jpg`,
        "/current/missing.jpg"
      );
      store.updateRequest({ isDir: false } as Resource);
      expect(store.restoreGallerySelection(`${folder}/item-3`)).toBe(false);
      store.updateRequest({
        ...directory,
        items: [...directory.items].reverse().map((item, index) => ({
          ...item,
          index,
        })),
      });
      expect(store.restoreGallerySelection(`${folder}/`)).toBe(true);
      expect(store.selected).toEqual([2]);
      expect(store.multiple).toBe(true);
      expect(store.gallerySelection).toBeNull();
    }
  );

  it("discards slideshow selection when visiting another folder", () => {
    const store = listing(4);
    store.toggleGallerySelection("/files/current/item-1", "/current/item-1");
    expect(store.restoreGallerySelection("/files/other/")).toBe(false);
    expect(store.selected).toEqual([]);
    expect(store.gallerySelection).toBeNull();
  });
});
