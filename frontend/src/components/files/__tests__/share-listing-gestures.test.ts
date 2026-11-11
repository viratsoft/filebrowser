import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { effectScope, reactive } from "vue";
import ListingItem from "../ListingItem.vue";

const harness = vi.hoisted(() => ({
  push: vi.fn(),
  getStore: vi.fn(),
  unmount: [] as Array<() => void>,
}));
vi.mock("vue", async (original) => ({
  ...(await original<typeof import("vue")>()),
  useSSRContext: () => ({ modules: new Set<string>() }),
  inject: () => vi.fn(),
  onBeforeUnmount: (callback: () => void) => harness.unmount.push(callback),
}));
vi.mock("vue-router", () => ({
  useRouter: () => ({ push: harness.push }),
}));
vi.mock("@/stores/auth", () => ({ useAuthStore: () => ({ user: null }) }));
vi.mock("@/stores/file", () => ({ useFileStore: harness.getStore }));
vi.mock("@/stores/layout", () => ({ useLayoutStore: () => ({}) }));
vi.mock("@/utils/constants", () => ({ enableThumbs: true }));
vi.mock("@/utils", () => ({ filesize: vi.fn() }));
vi.mock("@/utils/upload", () => ({}));
vi.mock("@/api", () => ({ files: {} }));

let scope: ReturnType<typeof effectScope>;
const makeStore = () =>
  reactive({
    selected: [] as number[],
    multiple: false,
    get selectedCount() {
      return this.selected.length;
    },
    removeSelected(index: number) {
      this.selected = this.selected.filter((selected) => selected !== index);
    },
  });
let store: ReturnType<typeof makeStore>;
const click = {
  ctrlKey: false,
  metaKey: false,
  shiftKey: false,
  preventDefault: vi.fn(),
} as unknown as MouseEvent;
const touch = (clientX: number) =>
  ({ touches: [{ clientX, clientY: 0 }] }) as unknown as TouchEvent;

function row(index: number) {
  const url = `/share/hash/photo-${index}.jpg`;
  return scope.run(() =>
    (ListingItem as any).setup(
      { name: `photo-${index}.jpg`, type: "image", url, index, readOnly: true },
      { expose: vi.fn() }
    )
  );
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal("window", { setTimeout, clearTimeout });
  store = makeStore();
  scope = effectScope();
  harness.getStore.mockReturnValue(store);
  harness.push.mockReset();
});
afterEach(() => {
  for (const callback of harness.unmount.splice(0)) callback();
  scope.stop();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("public share file gestures", () => {
  it("opens the gallery on the first tap", () => {
    row(0).itemClick(click);
    expect(harness.push).toHaveBeenCalledExactlyOnceWith({
      path: "/share/hash/photo-0.jpg",
    });
    expect(store.selected).toEqual([]);
  });

  it("long press selects without opening and subsequent taps toggle items", () => {
    const first = row(0);
    const second = row(1);
    const release = {
      cancelable: true,
      preventDefault: vi.fn(),
    } as unknown as TouchEvent;
    first.handleTouchStart(touch(0));
    vi.advanceTimersByTime(500);
    first.handleTouchEnd(release);
    expect(release.preventDefault).toHaveBeenCalled();
    first.itemClick(click);
    expect(store.multiple).toBe(true);
    expect(store.selected).toEqual([0]);
    second.itemClick(click);
    expect(store.selected).toEqual([0, 1]);
    second.itemClick(click);
    expect(store.selected).toEqual([0]);
    expect(harness.push).not.toHaveBeenCalled();
  });

  it("cancels long press when the user scrolls", () => {
    const first = row(0);
    first.handleTouchStart(touch(0));
    first.handleTouchMove(touch(15));
    vi.advanceTimersByTime(500);
    expect(store.multiple).toBe(false);
    expect(store.selected).toEqual([]);
    expect(harness.push).not.toHaveBeenCalled();
  });
});
