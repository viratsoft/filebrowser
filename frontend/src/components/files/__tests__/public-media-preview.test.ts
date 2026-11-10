import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { effectScope, nextTick, reactive } from "vue";
import PublicMediaPreview from "../PublicMediaPreview.vue";

const harness = vi.hoisted(() => ({
  unmount: [] as Array<() => void>,
  fetch: vi.fn(),
  fetchImage: vi.fn(),
  push: vi.fn(),
}));
vi.mock("vue", async (original) => ({
  ...(await original<typeof import("vue")>()),
  useSSRContext: () => ({ modules: new Set<string>() }),
  inject: () => vi.fn(),
  onMounted: vi.fn(),
  onBeforeUnmount: (callback: () => void) => harness.unmount.push(callback),
}));
vi.mock("vue-router", () => ({ useRouter: () => ({ push: harness.push }) }));
vi.mock("vue-i18n", () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock("@/stores/layout", () => ({
  useLayoutStore: () => ({ loading: false, currentPrompt: null }),
}));
vi.mock("@/api", () => ({
  pub: {
    fetch: harness.fetch,
    fetchImage: harness.fetchImage,
    getDownloadURL: (file: Resource) => `original:${file.hash}:${file.path}`,
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
  harness.unmount.length = 0;
  harness.fetch.mockReset();
  harness.fetchImage.mockReset();
  harness.push.mockReset();
  harness.fetch.mockResolvedValue({
    isDir: true,
    items: [photo(), photo("b.jpg")],
  });
  vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:private-preview");
  vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
  vi.stubGlobal("window", { removeEventListener: vi.fn() });
});
afterEach(() => {
  for (const callback of harness.unmount.splice(0)) callback();
  scope.stop();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("public gallery HD preference", () => {
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
