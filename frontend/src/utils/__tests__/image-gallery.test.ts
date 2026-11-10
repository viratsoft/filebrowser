import { describe, expect, it } from "vitest";
import { ImageGalleryController } from "../image-gallery";

function gallery(
  width = 400,
  height = 800,
  imageWidth = 1600,
  imageHeight = 1200
) {
  const model = new ImageGalleryController();
  model.setViewport(width, height);
  model.setImage(imageWidth, imageHeight);
  return model;
}

function tap(
  model: ImageGalleryController,
  x: number,
  y: number,
  time: number
) {
  model.pointerDown(1, { x, y }, time, "touch");
  return model.pointerUp(1, { x, y }, time + 30);
}

describe("image gallery geometry", () => {
  it("fits and centers landscape and portrait images without cropping", () => {
    const landscape = gallery();
    expect(landscape.transform).toMatchObject({
      width: 400,
      height: 300,
      left: 0,
      top: 250,
      scale: 1,
    });
    const portrait = gallery(400, 800, 1200, 3000);
    expect(portrait.transform).toMatchObject({
      width: 320,
      height: 800,
      left: 40,
      top: 0,
      scale: 1,
    });
  });

  it("keeps the tapped image point under the finger while zooming", () => {
    const model = gallery(400, 400, 1600, 1600);
    const point = { x: 280, y: 240 };
    model.doubleTap(point);
    expect(model.scale).toBe(2.5);
    // At fit, the point lies 70% across and 60% down the photo.
    expect(
      model.transform.left + model.transform.width * model.scale * 0.7
    ).toBeCloseTo(point.x);
    expect(
      model.transform.top + model.transform.height * model.scale * 0.6
    ).toBeCloseTo(point.y);
    model.doubleTap(point);
    expect(model.transform).toMatchObject({
      scale: 1,
      x: 0,
      y: 0,
      left: 0,
      top: 0,
    });
  });

  it("ignores a double-tap in the letterbox instead of zooming empty space", () => {
    const model = gallery();
    model.doubleTap({ x: 200, y: 40 });
    expect(model.scale).toBe(1);
  });

  it("clamps zoom and pan, keeping a short image centered vertically", () => {
    const model = gallery();
    model.zoomAt({ x: 200, y: 400 }, 2);
    model.pointerDown(1, { x: 200, y: 400 }, 0, "touch");
    model.pointerMove(1, { x: 2000, y: 2000 });
    expect(model.x).toBe(200);
    expect(model.y).toBe(0);
    expect(model.pointerUp(1, { x: 2000, y: 2000 }, 300)).toBeNull();
    model.zoomAt({ x: 200, y: 400 }, 100);
    expect(model.scale).toBe(8);
    model.zoomAt({ x: 200, y: 400 }, -1);
    expect(model.transform).toMatchObject({ scale: 1, x: 0, y: 0 });
  });

  it("preserves zoom on rotation, bounds pan, and cancels the old gesture", () => {
    const model = gallery(400, 800, 1600, 1600);
    model.zoomAt({ x: 280, y: 400 }, 2.5);
    model.pointerDown(1, { x: 280, y: 400 }, 0, "touch");
    model.setViewport(800, 400);
    expect(model.scale).toBe(2.5);
    expect(model.x).toBe(-100);
    expect(model.isInteracting).toBe(false);
    expect(model.pointerUp(1, { x: 0, y: 400 }, 100)).toBeNull();
  });

  it("does not clear a tap or an active gesture on an unchanged resize report", () => {
    const model = gallery();
    tap(model, 200, 400, 0);
    model.setViewport(400, 800);
    tap(model, 200, 400, 200);
    expect(model.scale).toBe(2.5);
    model.pointerDown(1, { x: 200, y: 400 }, 500, "touch");
    model.setViewport(400, 800);
    expect(model.hasPointer(1)).toBe(true);
  });

  it("resets the previous photo's zoom and contacts when the source changes", () => {
    const model = gallery();
    model.zoomAt({ x: 200, y: 400 }, 5);
    model.pointerDown(1, { x: 200, y: 400 }, 0, "touch");
    model.setImage(1200, 1600);
    expect(model.transform).toMatchObject({ scale: 1, x: 0, y: 0 });
    expect(model.isInteracting).toBe(false);
  });

  it("ignores invalid geometry and coordinates without introducing NaN transforms", () => {
    const model = gallery();
    model.zoomAt({ x: NaN, y: 0 }, 4);
    model.zoomAt({ x: 100, y: 400 }, Infinity);
    expect(model.scale).toBe(1);
    model.pointerDown(1, { x: 200, y: 400 }, 0, "touch");
    expect(model.pointerUp(1, { x: NaN, y: 0 }, 100)).toBeNull();
    expect(model.isInteracting).toBe(false);
    model.setImage(NaN, Infinity);
    expect(model.fitted).toEqual({ width: 0, height: 0 });
    model.setViewport(0, 0);
    expect(model.pointerDown(1, { x: 200, y: 400 }, 200, "touch")).toBe(false);
  });
});

describe("image gallery gestures", () => {
  it("keeps toolbar taps and swipes available when an image is still loading or failed", () => {
    const model = gallery();
    model.setImage(0, 0);
    tap(model, 200, 400, 0);
    expect(model.consumeSingleTap(380)).toBe(true);
    model.doubleTap({ x: 200, y: 400 });
    expect(model.scale).toBe(1);
    model.pointerDown(1, { x: 300, y: 400 }, 600, "touch");
    expect(model.pointerUp(1, { x: 50, y: 400 }, 800)).toBe("next");
  });

  it("defers single-tap until double-tap is ruled out and consumes it only once", () => {
    const model = gallery();
    tap(model, 200, 400, 0);
    expect(model.singleTapDelay(30)).toBe(350);
    expect(model.consumeSingleTap(379)).toBe(false);
    expect(model.consumeSingleTap(380)).toBe(true);
    expect(model.consumeSingleTap(500)).toBe(false);
  });

  it("double-tap zoom never also emits a pending toolbar toggle", () => {
    const model = gallery();
    tap(model, 200, 400, 0);
    model.pointerDown(1, { x: 200, y: 400 }, 180, "touch");
    expect(model.consumeSingleTap(400)).toBe(false);
    model.pointerUp(1, { x: 200, y: 400 }, 210);
    expect(model.scale).toBe(2.5);
    expect(model.singleTapDelay(600)).toBeNull();
    expect(model.consumeSingleTap(600)).toBe(false);
  });

  it("allows toolbar taps in the letterbox without zooming the photo", () => {
    const model = gallery();
    tap(model, 200, 30, 0);
    expect(model.consumeSingleTap(380)).toBe(true);
    expect(model.scale).toBe(1);
  });

  it("cancels a pending toolbar tap on pinch, pan, source change, or cancellation", () => {
    const model = gallery();
    tap(model, 200, 400, 0);
    model.pointerDown(1, { x: 100, y: 400 }, 150, "touch");
    model.pointerDown(2, { x: 300, y: 400 }, 150, "touch");
    model.pointerUp(1, { x: 100, y: 400 }, 200);
    model.pointerUp(2, { x: 300, y: 400 }, 200);
    expect(model.consumeSingleTap(600)).toBe(false);
    tap(model, 200, 400, 700);
    model.cancelGesture();
    expect(model.consumeSingleTap(1200)).toBe(false);
    tap(model, 200, 400, 1300);
    model.setImage(1600, 1200);
    expect(model.consumeSingleTap(1800)).toBe(false);
    model.zoomAt({ x: 200, y: 400 }, 2.5);
    tap(model, 200, 400, 1900);
    model.pointerDown(1, { x: 200, y: 400 }, 2100, "touch");
    model.pointerUp(1, { x: 300, y: 400 }, 2200);
    expect(model.consumeSingleTap(2700)).toBe(false);
  });

  it("keeps most of the fitted photo visible even during an extreme held swipe", () => {
    const model = gallery();
    model.pointerDown(1, { x: 200, y: 400 }, 0, "touch");
    model.pointerMove(1, { x: 2000, y: 400 });
    expect(model.transform.left).toBe(100);
    model.pointerMove(1, { x: -2000, y: 400 });
    expect(model.transform.left).toBe(-100);
    expect(model.pointerUp(1, { x: -2000, y: 400 }, 2000)).toBeNull();
    expect(model.transform.left).toBe(0);
  });

  it.each([
    [1600, 1200],
    [1200, 3000],
    [300, 200],
  ])(
    "never pans beyond any image border for a %s by %s photo",
    (width, height) => {
      const model = gallery(400, 800, width, height);
      for (const scale of [1.1, 2.5, 8]) {
        model.zoomAt({ x: 200, y: 400 }, scale);
        model.pointerDown(1, { x: 200, y: 400 }, 0, "touch");
        for (const x of [-10000, 10000]) {
          for (const y of [-10000, 10000]) {
            model.pointerMove(1, { x, y });
            const image = model.transform;
            const renderedWidth = image.width * image.scale;
            const renderedHeight = image.height * image.scale;
            if (renderedWidth >= 400) {
              expect(image.left).toBeLessThanOrEqual(0);
              expect(image.left + renderedWidth).toBeGreaterThanOrEqual(400);
            } else expect(image.left).toBeCloseTo((400 - renderedWidth) / 2);
            if (renderedHeight >= 800) {
              expect(image.top).toBeLessThanOrEqual(0);
              expect(image.top + renderedHeight).toBeGreaterThanOrEqual(800);
            } else expect(image.top).toBeCloseTo((800 - renderedHeight) / 2);
          }
        }
        model.cancelGesture();
      }
    }
  );

  it.each([
    [300, 60, "next"],
    [60, 300, "previous"],
  ] as const)(
    "navigates a normal-size horizontal swipe from %s to %s",
    (start, end, expected) => {
      const model = gallery();
      model.pointerDown(1, { x: start, y: 400 }, 0, "touch");
      model.pointerMove(1, { x: end, y: 410 });
      expect(model.transform.left).toBeCloseTo((end - start) * 0.35);
      expect(model.pointerUp(1, { x: end, y: 410 }, 200)).toBe(expected);
      expect(model.transform.left).toBe(0);
    }
  );

  it.each([
    [260, 410, 100], // Too short.
    [60, 700, 200], // Predominantly vertical.
    [60, 410, 2000], // Long hold/drag.
  ])("does not navigate a short, vertical, or held gesture", (x, y, time) => {
    const model = gallery();
    model.pointerDown(1, { x: 300, y: 400 }, 0, "touch");
    expect(model.pointerUp(1, { x, y }, time)).toBeNull();
    expect(model.transform.left).toBe(0);
  });

  it("pans a zoomed photo without switching even when dragged at its boundary", () => {
    const model = gallery();
    model.zoomAt({ x: 200, y: 400 }, 2.5);
    model.pointerDown(1, { x: 300, y: 400 }, 0, "touch");
    expect(model.pointerUp(1, { x: -500, y: 400 }, 200)).toBeNull();
    expect(model.x).toBe(-300);
  });

  it("zooms on the second nearby tap, then returns to fit on the next double-tap", () => {
    const model = gallery();
    tap(model, 200, 400, 0);
    expect(model.scale).toBe(1);
    tap(model, 202, 402, 180);
    expect(model.scale).toBe(2.5);
    tap(model, 202, 402, 600);
    tap(model, 202, 402, 780);
    expect(model.scale).toBe(1);
  });

  it("does not treat a dragged finger, distant taps, or old taps as double-tap", () => {
    const model = gallery();
    tap(model, 100, 400, 0);
    tap(model, 300, 400, 180);
    expect(model.scale).toBe(1);
    tap(model, 300, 400, 800);
    expect(model.scale).toBe(1);
    model.pointerDown(1, { x: 300, y: 400 }, 900, "touch");
    model.pointerMove(1, { x: 320, y: 400 });
    model.pointerUp(1, { x: 300, y: 400 }, 950);
    tap(model, 300, 400, 1100);
    expect(model.scale).toBe(1);
  });

  it("anchors pinch zoom to both fingers and supports moving the pinch center", () => {
    const model = gallery(400, 400, 1600, 1600);
    model.pointerDown(1, { x: 150, y: 200 }, 0, "touch");
    model.pointerDown(2, { x: 250, y: 200 }, 0, "touch");
    model.pointerMove(1, { x: 100, y: 200 });
    model.pointerMove(2, { x: 300, y: 200 });
    expect(model.scale).toBe(2);
    expect(model.x).toBe(0);
    model.pointerMove(1, { x: 150, y: 200 });
    model.pointerMove(2, { x: 350, y: 200 });
    expect(model.scale).toBe(2);
    expect(model.x).toBe(50);
    expect(model.pointerUp(1, { x: 150, y: 200 }, 100)).toBeNull();
    expect(model.pointerUp(2, { x: 350, y: 200 }, 100)).toBeNull();
  });

  it("never navigates after lifting one finger of a pinch, even back at fit", () => {
    const model = gallery();
    model.pointerDown(1, { x: 100, y: 400 }, 0, "touch");
    model.pointerDown(2, { x: 300, y: 400 }, 0, "touch");
    model.pointerMove(2, { x: 200, y: 400 });
    model.pointerUp(1, { x: 100, y: 400 }, 100);
    expect(model.pointerUp(2, { x: 0, y: 400 }, 200)).toBeNull();
    expect(model.scale).toBe(1);
    // The next independent swipe still works.
    model.pointerDown(1, { x: 300, y: 400 }, 300, "touch");
    expect(model.pointerUp(1, { x: 50, y: 400 }, 500)).toBe("next");
  });

  it("handles a third finger and cancellation without phantom taps or navigation", () => {
    const model = gallery();
    model.pointerDown(1, { x: 100, y: 400 }, 0, "touch");
    model.pointerDown(2, { x: 200, y: 400 }, 0, "touch");
    model.pointerDown(3, { x: 300, y: 400 }, 0, "touch");
    expect(model.pointerUp(1, { x: 100, y: 400 }, 100)).toBeNull();
    model.cancelGesture();
    expect(model.isInteracting).toBe(false);
    expect(model.pointerUp(3, { x: 0, y: 400 }, 200)).toBeNull();
    tap(model, 200, 400, 300);
    expect(model.scale).toBe(1);
  });

  it("retains desktop double-click zoom and pan without mouse swipe navigation", () => {
    const model = gallery();
    for (const time of [0, 150]) {
      model.pointerDown(1, { x: 200, y: 400 }, time, "mouse");
      expect(model.pointerUp(1, { x: 200, y: 400 }, time + 30)).toBeNull();
    }
    expect(model.scale).toBe(2.5);
    model.pointerDown(1, { x: 200, y: 400 }, 400, "mouse");
    expect(model.pointerDown(2, { x: 250, y: 400 }, 410, "touch")).toBe(false);
    expect(model.pointerUp(1, { x: 0, y: 400 }, 500)).toBeNull();
    expect(model.x).toBe(-200);
    model.reset();
    model.pointerDown(1, { x: 300, y: 400 }, 600, "mouse");
    expect(model.pointerUp(1, { x: 0, y: 400 }, 700)).toBeNull();
    expect(model.x).toBe(0);
  });
});
