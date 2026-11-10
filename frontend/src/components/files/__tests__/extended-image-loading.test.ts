import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { effectScope, markRaw, nextTick, reactive } from "vue";
import ExtendedImage from "../ExtendedImage.vue";

const harness = vi.hoisted(() => ({
  mounted: [] as Array<() => void>,
  unmounted: [] as Array<() => void>,
  decode: vi.fn(),
  requests: [] as any[],
  images: [] as any[],
}));
vi.mock("vue", async (original) => ({
  ...(await original<typeof import("vue")>()),
  onMounted: (callback: () => void) => harness.mounted.push(callback),
  onBeforeUnmount: (callback: () => void) => harness.unmounted.push(callback),
}));
vi.mock("vue-i18n", () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock("utif", () => ({
  default: {
    _imgLoaded: harness.decode,
    _xhrs: harness.requests,
    _imgs: harness.images,
  },
}));

class FakeRequest {
  static instances: FakeRequest[] = [];
  status = 200;
  responseType = "";
  onload: ((event: { target: FakeRequest }) => void) | null = null;
  onerror: (() => void) | null = null;
  open = vi.fn();
  send = vi.fn();
  abort = vi.fn();
  constructor() {
    FakeRequest.instances.push(this);
  }
  finish() {
    this.onload?.({ target: this });
  }
}

let scope: ReturnType<typeof effectScope>;
let removeWindowListener: ReturnType<typeof vi.fn>;
let removeDocumentListener: ReturnType<typeof vi.fn>;

beforeEach(() => {
  harness.mounted.length = 0;
  harness.unmounted.length = 0;
  harness.requests.length = 0;
  harness.images.length = 0;
  harness.decode.mockReset();
  FakeRequest.instances = [];
  removeWindowListener = vi.fn();
  removeDocumentListener = vi.fn();
  vi.stubGlobal("window", {
    PointerEvent: function () {},
    performance: { now: () => 0 },
    addEventListener: vi.fn(),
    removeEventListener: removeWindowListener,
  });
  vi.stubGlobal("document", {
    baseURI: "https://example.test/files/",
    addEventListener: vi.fn(),
    removeEventListener: removeDocumentListener,
  });
  vi.stubGlobal("XMLHttpRequest", FakeRequest);
  vi.stubGlobal("ResizeObserver", undefined);
  scope = effectScope();
});
afterEach(() => {
  for (const callback of harness.unmounted.splice(0)) callback();
  scope.stop();
  vi.unstubAllGlobals();
});

async function openImage(src: string) {
  const props = reactive({
    src,
    alt: "photo",
    classList: [],
    zoomStep: 0.25,
    canGoNext: true,
    canGoPrevious: true,
  });
  const image = markRaw({ src: "", naturalWidth: 1600, naturalHeight: 1200 });
  const component = scope.run(() =>
    (ExtendedImage as any).setup(props, { emit: vi.fn(), expose: vi.fn() })
  );
  component.container.value = {
    clientWidth: 400,
    clientHeight: 800,
    getBoundingClientRect: () => ({ left: 0, top: 0 }),
  };
  component.imgex.value = image;
  for (const callback of harness.mounted) callback();
  await nextTick();
  return { props, image, component };
}

describe("gallery image loading and cleanup", () => {
  it("lets the browser decode preview bytes even when the URL ends in .tif", async () => {
    const { image, component } = await openImage(
      "/api/preview/big/photo.tif?key=123"
    );
    expect(image.src).toBe("/api/preview/big/photo.tif?key=123");
    component.onLoad({ target: image });
    expect(component.ready.value).toBe(true);
    expect(FakeRequest.instances).toHaveLength(0);
    expect(harness.decode).not.toHaveBeenCalled();
  });

  it("uses the same authorized source for a one-shot TIFF fallback and cleans its registry", async () => {
    const { image, component } = await openImage(
      "/api/raw/photo.tif?inline=true"
    );
    component.onError({ target: image });
    const request = FakeRequest.instances[0];
    expect(request.open).toHaveBeenCalledWith(
      "GET",
      "/api/raw/photo.tif?inline=true"
    );
    harness.decode.mockImplementation(() => {
      throw new Error("invalid TIFF");
    });
    request.finish();
    expect(component.failed.value).toBe(true);
    expect(harness.requests).toHaveLength(0);
    expect(harness.images).toHaveLength(0);
    expect(FakeRequest.instances).toHaveLength(1);
  });

  it("aborts a previous request and ignores its late response after a source change", async () => {
    const { props, image, component } = await openImage("/api/raw/old.tif");
    component.onError({ target: image });
    const oldRequest = FakeRequest.instances[0];
    props.src = "/api/preview/big/new.jpg";
    await nextTick();
    await nextTick();
    oldRequest.finish();
    expect(oldRequest.abort).toHaveBeenCalledOnce();
    expect(image.src).toBe("/api/preview/big/new.jpg");
    expect(harness.decode).not.toHaveBeenCalled();
    expect(component.failed.value).toBe(false);
  });

  it("ignores detached image events and late responses after closing the viewer", async () => {
    const { image, component } = await openImage("/api/raw/photo.tif");
    component.onError({ target: {} });
    expect(FakeRequest.instances).toHaveLength(0);
    component.onError({ target: image });
    const request = FakeRequest.instances[0];
    for (const callback of harness.unmounted.splice(0)) callback();
    request.finish();
    expect(request.abort).toHaveBeenCalledOnce();
    expect(harness.decode).not.toHaveBeenCalled();
    expect(removeWindowListener).toHaveBeenCalledWith(
      "pointermove",
      expect.any(Function)
    );
    expect(removeDocumentListener).toHaveBeenCalledWith(
      "visibilitychange",
      expect.any(Function)
    );
  });
});
