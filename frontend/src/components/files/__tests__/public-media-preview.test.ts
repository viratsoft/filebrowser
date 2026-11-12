import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { effectScope, nextTick, reactive } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import PublicMediaPreview from "../PublicMediaPreview.vue";

const harness = vi.hoisted(() => ({
  mount: [] as Array<() => void>,
  unmount: [] as Array<() => void>,
  fetch: vi.fn(),
  fetchImage: vi.fn(),
  replace: vi.fn(),
}));
vi.mock("vue", async (original) => ({
  ...(await original<typeof import("vue")>()),
  useSSRContext: () => ({ modules: new Set<string>() }),
  inject: () => vi.fn(),
  onMounted: (callback: () => void) => harness.mount.push(callback),
  onBeforeUnmount: (callback: () => void) => harness.unmount.push(callback),
}));
vi.mock("vue-router", async (original) => ({
  ...(await original<typeof import("vue-router")>()),
  useRouter: () => ({ replace: harness.replace }),
}));
vi.mock("vue-i18n", () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock("@/stores/layout", () => ({
  useLayoutStore: () => ({ loading: false, currentPrompt: null }),
}));
vi.mock("@/api", () => ({
  pub: {
    fetch: harness.fetch,
    fetchImage: harness.fetchImage,
    getDownloadURL: (file: Resource, inline = false) =>
      `${inline ? "original" : "download"}:${file.hash}:${file.path}`,
    getImagePreviewURL: (file: Resource) => `preview:${file.hash}:${file.path}`,
  },
}));
vi.mock("@/components/header/HeaderBar.vue", () => ({ default: {} }));
vi.mock("@/components/header/Action.vue", () => ({ default: {} }));
vi.mock("../ExtendedImage.vue", () => ({ default: {} }));
vi.mock("../VideoPlayer.vue", () => ({ default: {} }));

let scope: ReturnType<typeof effectScope>;
const photo = (name = "a.jpg") =>
  ({
    name,
    path: `/${name}`,
    url: `/share/hash/${name}`,
    hash: "hash",
    type: "image",
    allowImagePreview: true,
  }) as Resource;
async function setup(password = "") {
  const props = reactive({ file: photo(), original: false, password });
  const emit = vi.fn();
  const view = scope.run(() =>
    (PublicMediaPreview as any).setup(props, { emit, expose: vi.fn() })
  );
  await nextTick();
  return { props, emit, view };
}
beforeEach(() => {
  scope = effectScope();
  harness.mount.length = 0;
  harness.unmount.length = 0;
  harness.fetch.mockReset();
  harness.fetchImage.mockReset();
  harness.replace.mockReset();
  harness.fetch.mockResolvedValue({
    isDir: true,
    items: [photo(), photo("b.jpg")],
  });
  vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:private-preview");
  vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
  vi.stubGlobal("window", {
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  });
});
afterEach(() => {
  for (const callback of harness.unmount.splice(0)) callback();
  scope.stop();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("public gallery navigation and quality", () => {
  it("locks background scrolling for the viewer lifetime", async () => {
    const classList = new Set(["dark"]);
    vi.stubGlobal("document", {
      documentElement: {
        classList: {
          add: (name: string) => classList.add(name),
          remove: (name: string) => classList.delete(name),
        },
      },
    });
    await setup();
    expect(classList.has("share-gallery-open")).toBe(false);
    for (const callback of harness.mount.splice(0)) callback();
    expect(classList.has("share-gallery-open")).toBe(true);
    for (const callback of harness.unmount.splice(0)) callback();
    expect([...classList]).toEqual(["dark"]);
  });

  it("returns directly to the folder on Back after multiple slides", async () => {
    const folder = "/share/hash/";
    const second: Resource = { ...photo("b.mp4"), type: "video" };
    const third = photo("c.jpg");
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/share/:path(.*)*", component: {} }],
    });
    harness.replace.mockImplementation((location) => router.replace(location));
    harness.fetch.mockResolvedValue({
      isDir: true,
      items: [photo(), second, third],
    });
    await router.push(folder);
    await router.push(photo().url);

    const { props, view } = await setup();
    await view.go(view.next.value);
    expect(router.currentRoute.value.path).toBe(second.url);
    props.file = second;
    await nextTick();
    expect(view.currentIndex.value + 1).toBe(2);
    expect(view.siblings.value).toHaveLength(3);
    await view.go(view.next.value);
    expect(router.currentRoute.value.path).toBe(third.url);
    props.file = third;
    await nextTick();
    expect(view.source.value).toBe("preview:hash:/c.jpg");

    const back = new Promise<void>((resolve) => {
      const unsubscribe = router.afterEach(() => {
        unsubscribe();
        resolve();
      });
    });
    router.back();
    await back;
    expect(router.currentRoute.value.path).toBe(folder);
  });

  it("keeps the selected quality on next images and across a video slide", async () => {
    const { props, emit, view } = await setup();
    expect(view.source.value).toBe("preview:hash:/a.jpg");
    view.toggleQuality();
    expect(emit).toHaveBeenCalledWith("update:original", true);
    props.original = true;
    await nextTick();
    expect(view.source.value).toBe("original:hash:/a.jpg");
    props.file = { ...photo("clip.mp4"), type: "video" };
    await nextTick();
    props.file = photo("b.jpg");
    await nextTick();
    expect(view.source.value).toBe("original:hash:/b.jpg");
    view.toggleQuality();
    props.original = false;
    await nextTick();
    expect(view.source.value).toBe("preview:hash:/b.jpg");
    expect(harness.fetch).toHaveBeenCalledTimes(1);
  });

  it("clears the old slide count when share credentials change", async () => {
    const { props, view } = await setup();
    expect(view.siblings.value).toHaveLength(2);
    harness.fetch.mockRejectedValueOnce(new Error("unauthorized"));
    props.file = { ...props.file, token: "different-share-token" };
    await nextTick();
    expect(view.siblings.value).toHaveLength(0);
    expect(view.currentIndex.value).toBe(-1);
  });

  it("offers the original download for readable share media slides", async () => {
    const { props, view } = await setup();
    expect(view.downloadURL.value).toBe("download:hash:/a.jpg");
    props.original = true;
    await nextTick();
    expect(view.downloadURL.value).toBe("download:hash:/a.jpg");
    props.file = { ...photo("video.mp4"), type: "video" };
    await nextTick();
    expect(view.downloadURL.value).toBe("download:hash:/video.mp4");
    props.file = { ...photo("audio.mp3"), type: "audio" };
    await nextTick();
    expect(view.downloadURL.value).toBe("download:hash:/audio.mp3");
    props.file = { ...photo(), uploadOnly: true };
    await nextTick();
    expect(view.downloadURL.value).toBe("");
  });

  it("does not issue media/listing requests for upload-only data", async () => {
    const { props, view } = await setup();
    harness.fetch.mockClear();
    props.file = { ...photo("private.jpg"), uploadOnly: true };
    await nextTick();
    expect(view.mediaURL.value).toBe("");
    expect(view.source.value).toBe("");
    expect(harness.fetch).not.toHaveBeenCalled();
    expect(harness.fetchImage).not.toHaveBeenCalled();
  });

  it("uses only a current header-authenticated image and revokes its object URL on close", async () => {
    let finishOld!: (blob: Blob) => void;
    let finishNew!: (blob: Blob) => void;
    harness.fetchImage
      .mockReturnValueOnce(
        new Promise<Blob>((resolve) => {
          finishOld = resolve;
        })
      )
      .mockReturnValueOnce(
        new Promise<Blob>((resolve) => {
          finishNew = resolve;
        })
      );
    const { props, view } = await setup("secret");
    const oldSignal = harness.fetchImage.mock.calls[0][3] as AbortSignal;
    props.original = true;
    await nextTick();
    expect(oldSignal.aborted).toBe(true);
    finishOld(new Blob(["old"]));
    await nextTick();
    expect(URL.createObjectURL).not.toHaveBeenCalled();
    finishNew(new Blob(["new"]));
    await nextTick();
    expect(view.source.value).toBe("blob:private-preview");
    for (const callback of harness.unmount.splice(0)) callback();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:private-preview");
  });
});
