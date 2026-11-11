import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { effectScope } from "vue";
import GalleryStage from "../GalleryStage.vue";

const unmount = vi.hoisted(() => [] as Array<() => void>);
vi.mock("vue", async (original) => ({
  ...(await original<typeof import("vue")>()),
  useSSRContext: () => ({ modules: new Set<string>() }),
  onBeforeUnmount: (callback: () => void) => unmount.push(callback),
}));
vi.mock("vue-i18n", () => ({ useI18n: () => ({ t: (key: string) => key }) }));

class TouchTarget {
  constructor(private readonly control = false) {}
  closest() {
    return this.control ? this : null;
  }
}

let scope: ReturnType<typeof effectScope>;
const eventAt = (x: number, y: number, control = false) => {
  const touch = { identifier: 1, clientX: x, clientY: y };
  return {
    target: new TouchTarget(control),
    touches: [touch],
    changedTouches: [touch],
    cancelable: true,
    preventDefault: vi.fn(),
    stopPropagation: vi.fn(),
  } as unknown as TouchEvent;
};

function setup(overrides = {}) {
  const emit = vi.fn();
  const stage = scope.run(() =>
    (GalleryStage as any).setup(
      {
        swipeMedia: true,
        changing: false,
        canGoNext: true,
        canGoPrevious: true,
        ...overrides,
      },
      { expose: vi.fn(), emit }
    )
  );
  return { stage, emit };
}

beforeEach(() => {
  scope = effectScope();
  vi.useFakeTimers();
  vi.stubGlobal("Element", TouchTarget);
});

afterEach(() => {
  for (const callback of unmount.splice(0)) callback();
  scope.stop();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("gallery video swipes", () => {
  it.each([
    { offset: -75, direction: "next" },
    { offset: 75, direction: "previous" },
  ])(
    "moves the slide at horizontal offset $offset",
    ({ offset, direction }) => {
      const { stage, emit } = setup();
      stage.startSwipe(eventAt(100, 100));
      stage.moveSwipe(eventAt(100 + offset, 105));
      expect(stage.dragOffset.value).toBe(offset * 0.6);
      const end = eventAt(100 + offset, 105);
      stage.endSwipe(end);
      expect(emit).toHaveBeenCalledExactlyOnceWith(direction);
      expect(end.preventDefault).toHaveBeenCalled();
    }
  );

  it("lets player controls and vertical gestures handle their own events", () => {
    const { stage, emit } = setup();
    stage.startSwipe(eventAt(100, 100, true));
    stage.endSwipe(eventAt(25, 100, true));
    stage.startSwipe(eventAt(100, 100));
    stage.moveSwipe(eventAt(90, 150));
    const end = eventAt(80, 175);
    stage.endSwipe(end);
    expect(emit).not.toHaveBeenCalled();
    expect(end.preventDefault).not.toHaveBeenCalled();
  });

  it("does not navigate beyond the end of the gallery", () => {
    const { stage, emit } = setup({ canGoNext: false });
    stage.startSwipe(eventAt(100, 100));
    stage.moveSwipe(eventAt(25, 100));
    stage.endSwipe(eventAt(25, 100));
    expect(emit).not.toHaveBeenCalled();
    expect(stage.dragOffset.value).toBe(0);
  });
});
