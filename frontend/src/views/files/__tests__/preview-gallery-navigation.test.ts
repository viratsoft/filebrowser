import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Preview from "../Preview.vue";

const harness = vi.hoisted(() => ({
  route: { path: "/files/a/one.jpg", fullPath: "/files/a/one.jpg", query: {} },
  file: {
    req: { type: "image", extension: ".jpg", path: "/a/one.jpg" },
    oldReq: null,
  } as any,
  layout: { loading: false, currentPrompt: null },
  fetch: vi.fn(),
  replace: vi.fn(),
  error: vi.fn(),
  mounted: [] as Array<() => void>,
  unmounted: [] as Array<() => void>,
  watchers: [] as Array<(...args: any[]) => void>,
}));
vi.mock("vue", async (original) => ({
  ...(await original<typeof import("vue")>()),
  inject: () => harness.error,
  onMounted: (callback: () => void) => harness.mounted.push(callback),
  onBeforeUnmount: (callback: () => void) => harness.unmounted.push(callback),
  watch: (_source: unknown, callback: (...args: any[]) => void) =>
    harness.watchers.push(callback),
}));
vi.mock("@vueuse/core", () => ({
  useStorage: (_key: string, initial: unknown) => ({ value: initial }),
}));
vi.mock("@/stores/auth", () => ({
  useAuthStore: () => ({ user: { perm: { download: true } } }),
}));
vi.mock("@/stores/file", () => ({ useFileStore: () => harness.file }));
vi.mock("@/stores/layout", () => ({ useLayoutStore: () => harness.layout }));
vi.mock("@/api", () => ({
  files: {
    fetch: harness.fetch,
    getPreviewURL: (item: any) => `preview:${item.url}`,
    getDownloadURL: (item: any) => `raw:${item.url}`,
  },
}));
vi.mock("@/api/utils", () => ({ createURL: vi.fn() }));
vi.mock("@/utils/constants", () => ({ resizePreview: true }));
vi.mock("@/utils/theme", () => ({ getTheme: () => "light" }));
vi.mock("vue-reader", () => ({ VueReader: {} }));
vi.mock("vue-router", () => ({
  useRoute: () => harness.route,
  useRouter: () => ({ replace: harness.replace }),
}));
vi.mock("vue-i18n", () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock("@/components/header/HeaderBar.vue", () => ({ default: {} }));
vi.mock("@/components/header/Action.vue", () => ({ default: {} }));
vi.mock("@/components/files/ExtendedImage.vue", () => ({ default: {} }));
vi.mock("@/components/files/VideoPlayer.vue", () => ({ default: {} }));
vi.mock("@/components/files/CsvViewer.vue", () => ({ default: {} }));

const item = (folder: string, name: string, type = "image") => ({
  name,
  type,
  url: `/files/${folder}/${name}`,
});
const setup = () => (Preview as any).setup({}, { expose: vi.fn() });

beforeEach(() => {
  harness.route.path = "/files/a/one.jpg";
  harness.route.fullPath = "/files/a/one.jpg";
  harness.file.req = { type: "image", extension: ".jpg", path: "/a/one.jpg" };
  harness.file.oldReq = null;
  harness.layout.loading = false;
  harness.fetch.mockReset();
  harness.replace.mockReset();
  harness.error.mockReset();
  harness.mounted.length = 0;
  harness.unmounted.length = 0;
  harness.watchers.length = 0;
  vi.stubGlobal("window", {
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    setTimeout: vi.fn(() => 1),
  });
});
afterEach(() => {
  for (const callback of harness.unmounted.splice(0)) callback();
  vi.unstubAllGlobals();
});

describe("gallery navigation lifecycle", () => {
  it("ignores an old folder's late response after navigating into another folder", async () => {
    let resolveOld!: (value: any) => void;
    harness.fetch
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveOld = resolve;
        })
      )
      .mockResolvedValueOnce({
        items: [item("b", "one.jpg"), item("b", "two.mp4", "video")],
      });
    const view = setup();
    const old = view.updatePreview();
    const oldSignal = harness.fetch.mock.calls[0][1] as AbortSignal;
    harness.route.path = "/files/b/one.jpg";
    await view.updatePreview();
    expect(oldSignal.aborted).toBe(true);
    expect(view.nextLink.value).toBe("/files/b/two.mp4");
    resolveOld({ items: [item("a", "one.jpg"), item("a", "private.jpg")] });
    await old;
    expect(view.nextLink.value).toBe("/files/b/two.mp4");
    expect(view.nextRaw.value).toBe("");
  });

  it("aborts metadata loading on close without applying the late response", async () => {
    let finish!: (value: any) => void;
    harness.fetch.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      })
    );
    const view = setup();
    const pending = view.updatePreview();
    const signal = harness.fetch.mock.calls[0][1] as AbortSignal;
    for (const callback of harness.unmounted.splice(0)) callback();
    finish({ items: [item("a", "one.jpg"), item("a", "two.jpg")] });
    await pending;
    expect(signal.aborted).toBe(true);
    expect(view.nextLink.value).toBe("");
    expect(harness.error).not.toHaveBeenCalled();
  });

  it("does not include query parameters in the photo name", async () => {
    harness.route.fullPath = "/files/a/one.jpg?view=gallery";
    harness.fetch.mockResolvedValue({
      items: [item("a", "one.jpg"), item("a", "two.jpg")],
    });
    const view = setup();
    await view.updatePreview();
    expect(view.name.value).toBe("one.jpg");
    expect(view.nextLink.value).toBe("/files/a/two.jpg");
  });

  it("preserves video slides while preventing duplicate in-flight navigation", async () => {
    harness.fetch.mockResolvedValue({
      items: [item("a", "one.jpg"), item("a", "two.mp4", "video")],
    });
    let finish!: () => void;
    harness.replace.mockReturnValue(
      new Promise<void>((resolve) => {
        finish = resolve;
      })
    );
    const view = setup();
    await view.updatePreview();
    const pending = view.next();
    await view.next();
    expect(harness.replace).toHaveBeenCalledExactlyOnceWith({
      path: "/files/a/two.mp4",
    });
    finish();
    await pending;
    harness.layout.loading = true;
    await view.next();
    expect(harness.replace).toHaveBeenCalledTimes(1);
  });

  it("keeps hidden image controls across photos but restores them on a video/image transition", () => {
    const view = setup();
    view.showNav.value = false;
    const mediaChanged = harness.watchers[1];
    mediaChanged("image", "image");
    expect(view.showNav.value).toBe(false);
    mediaChanged("image", "video");
    expect(view.showNav.value).toBe(true);
    view.toggleGalleryNavigation();
    expect(view.showNav.value).toBe(false);
    mediaChanged("video", "image");
    expect(view.showNav.value).toBe(true);
  });
});
