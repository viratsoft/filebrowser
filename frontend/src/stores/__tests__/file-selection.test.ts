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
