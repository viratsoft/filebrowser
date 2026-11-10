export interface GalleryPoint {
  x: number;
  y: number;
}
export type GalleryPointerKind = "mouse" | "touch" | "pen";
export type GalleryNavigation = "next" | "previous" | null;

interface Contact extends GalleryPoint {
  kind: GalleryPointerKind;
}
interface SingleGesture {
  id: number;
  start: GalleryPoint;
  last: GalleryPoint;
  time: number;
  maxDistance: number;
  startedZoomed: boolean;
  kind: GalleryPointerKind;
}
interface PinchGesture {
  ids: [number, number];
  distance: number;
  center: GalleryPoint;
  scale: number;
  x: number;
  y: number;
}

const clamp = (value: number, min: number, max: number) =>
  Math.max(min, Math.min(max, value));
const distance = (a: GalleryPoint, b: GalleryPoint) =>
  Math.hypot(a.x - b.x, a.y - b.y);
const midpoint = (a: GalleryPoint, b: GalleryPoint) => ({
  x: (a.x + b.x) / 2,
  y: (a.y + b.y) / 2,
});

// Coordinates are local CSS pixels, never device pixels. Keeping gesture math
// separate from DOM events makes pinch/pan/navigation transitions testable.
export class ImageGalleryController {
  scale = 1;
  x = 0;
  y = 0;
  private swipeX = 0;
  private width = 0;
  private height = 0;
  private naturalWidth = 0;
  private naturalHeight = 0;
  private contacts = new Map<number, Contact>();
  private single: SingleGesture | null = null;
  private pinch: PinchGesture | null = null;
  private hadMultiple = false;
  private lastTap: {
    point: GalleryPoint;
    time: number;
    kind: GalleryPointerKind;
  } | null = null;
  readonly maxScale = 8;

  get fitted() {
    if (
      this.width <= 0 ||
      this.height <= 0 ||
      this.naturalWidth <= 0 ||
      this.naturalHeight <= 0
    )
      return { width: 0, height: 0 };
    const ratio = Math.min(
      1,
      this.width / this.naturalWidth,
      this.height / this.naturalHeight
    );
    return {
      width: this.naturalWidth * ratio,
      height: this.naturalHeight * ratio,
    };
  }

  get transform() {
    const fitted = this.fitted;
    return {
      ...fitted,
      scale: this.scale,
      x: this.x,
      y: this.y,
      left: (this.width - fitted.width * this.scale) / 2 + this.x + this.swipeX,
      top: (this.height - fitted.height * this.scale) / 2 + this.y,
    };
  }

  setImage(width: number, height: number) {
    this.naturalWidth = Number.isFinite(width) ? Math.max(0, width) : 0;
    this.naturalHeight = Number.isFinite(height) ? Math.max(0, height) : 0;
    this.reset();
  }

  setViewport(width: number, height: number) {
    const nextWidth = Number.isFinite(width) ? Math.max(0, width) : 0;
    const nextHeight = Number.isFinite(height) ? Math.max(0, height) : 0;
    // ResizeObserver also reports the initial layout. An unchanged report must
    // not interrupt a pinch or clear the first tap of a double-tap.
    if (this.width === nextWidth && this.height === nextHeight) return;
    const before = this.fitted;
    this.width = nextWidth;
    this.height = nextHeight;
    const after = this.fitted;
    if (before.width > 0) this.x *= after.width / before.width;
    if (before.height > 0) this.y *= after.height / before.height;
    this.cancelGesture();
    this.bound();
  }

  reset() {
    this.scale = 1;
    this.x = 0;
    this.y = 0;
    this.cancelGesture();
  }

  cancelGesture() {
    this.contacts.clear();
    this.single = null;
    this.pinch = null;
    this.hadMultiple = false;
    this.lastTap = null;
    this.swipeX = 0;
  }

  get isInteracting() {
    return this.contacts.size > 0;
  }

  hasPointer(id: number) {
    return this.contacts.has(id);
  }

  singleTapDelay(time: number): number | null {
    if (!this.lastTap || this.isInteracting) return null;
    const delay = this.lastTap.kind === "mouse" ? 500 : 350;
    return Math.max(0, delay - (time - this.lastTap.time));
  }

  consumeSingleTap(time: number) {
    if (this.singleTapDelay(time) !== 0) return false;
    this.lastTap = null;
    return true;
  }

  zoomAt(point: GalleryPoint, target: number) {
    if (
      !Number.isFinite(target) ||
      !this.validPoint(point) ||
      this.fitted.width === 0
    )
      return;
    this.lastTap = null;
    const next = clamp(target, 1, this.maxScale);
    const ratio = next / this.scale;
    const focal = this.centered(point);
    this.x = focal.x - (focal.x - this.x) * ratio;
    this.y = focal.y - (focal.y - this.y) * ratio;
    this.scale = next;
    this.bound();
  }

  doubleTap(point: GalleryPoint) {
    if (!this.insideImage(point)) return;
    if (this.scale > 1.01) {
      this.scale = 1;
      this.x = 0;
      this.y = 0;
    } else this.zoomAt(point, 2.5);
    this.lastTap = null;
  }

  pointerDown(
    id: number,
    point: GalleryPoint,
    time: number,
    kind: GalleryPointerKind
  ) {
    if (
      !this.validPoint(point) ||
      this.width <= 0 ||
      this.height <= 0 ||
      this.contacts.has(id)
    )
      return false;
    const first = this.contacts.values().next().value;
    if (first && first.kind !== kind) return false;
    this.contacts.set(id, { ...point, kind });
    if (this.contacts.size === 1) this.beginSingle(id, point, time, kind);
    else {
      this.hadMultiple = true;
      this.lastTap = null;
      this.beginPinch();
    }
    return true;
  }

  pointerMove(id: number, point: GalleryPoint) {
    const contact = this.contacts.get(id);
    if (!contact || !this.validPoint(point)) return;
    this.contacts.set(id, { ...point, kind: contact.kind });
    if (this.pinch) {
      const first = this.contacts.get(this.pinch.ids[0]);
      const second = this.contacts.get(this.pinch.ids[1]);
      if (!first || !second) return;
      const center = this.centered(midpoint(first, second));
      const next = clamp(
        (this.pinch.scale * distance(first, second)) / this.pinch.distance,
        1,
        this.maxScale
      );
      const ratio = next / this.pinch.scale;
      this.x = center.x - (this.pinch.center.x - this.pinch.x) * ratio;
      this.y = center.y - (this.pinch.center.y - this.pinch.y) * ratio;
      this.scale = next;
      this.bound();
    } else if (this.single?.id === id) {
      this.single.maxDistance = Math.max(
        this.single.maxDistance,
        distance(this.single.start, point)
      );
      if (this.scale > 1) {
        this.x += point.x - this.single.last.x;
        this.y += point.y - this.single.last.y;
        this.bound();
      } else if (this.single.kind !== "mouse" && !this.hadMultiple) {
        const dx = point.x - this.single.start.x;
        const dy = point.y - this.single.start.y;
        this.swipeX =
          Math.abs(dx) > Math.abs(dy) * 1.3
            ? clamp(dx * 0.35, -this.width * 0.25, this.width * 0.25)
            : 0;
      }
      this.single.last = { ...point };
    }
  }

  pointerUp(id: number, point: GalleryPoint, time: number): GalleryNavigation {
    if (!this.contacts.has(id)) return null;
    if (!this.validPoint(point)) {
      this.cancelGesture();
      return null;
    }
    this.pointerMove(id, point);
    const gesture = this.single;
    this.contacts.delete(id);
    if (this.contacts.size >= 2) {
      this.beginPinch();
      return null;
    }
    this.pinch = null;
    if (this.contacts.size === 1) {
      const [remainingId, remaining] = this.contacts.entries().next().value!;
      this.beginSingle(remainingId, remaining, time, remaining.kind);
      return null;
    }
    this.single = null;
    this.swipeX = 0;
    if (this.hadMultiple) {
      this.hadMultiple = false;
      this.lastTap = null;
      if (this.scale < 1.02) {
        this.scale = 1;
        this.x = 0;
        this.y = 0;
      }
      return null;
    }
    if (!gesture || !this.validPoint(point)) return null;
    const elapsed = time - gesture.time;
    const dx = point.x - gesture.start.x;
    const dy = point.y - gesture.start.y;
    const swipeDistance = Math.max(48, Math.min(100, this.width * 0.15));
    if (
      gesture.kind !== "mouse" &&
      !gesture.startedZoomed &&
      this.scale === 1 &&
      elapsed >= 0 &&
      elapsed <= 1200 &&
      Math.abs(dx) >= swipeDistance &&
      Math.abs(dx) > Math.abs(dy) * 1.3
    ) {
      this.lastTap = null;
      return dx < 0 ? "next" : "previous";
    }
    if (gesture.maxDistance <= 8 && elapsed >= 0 && elapsed <= 250) {
      const previous = this.lastTap;
      const delay = gesture.kind === "mouse" ? 500 : 350;
      const tolerance = gesture.kind === "mouse" ? 8 : 32;
      if (
        previous &&
        previous.kind === gesture.kind &&
        time >= previous.time &&
        time - previous.time <= delay &&
        distance(previous.point, point) <= tolerance &&
        this.insideImage(point) &&
        this.insideImage(previous.point)
      )
        this.doubleTap(point);
      else this.lastTap = { point: { ...point }, time, kind: gesture.kind };
    } else this.lastTap = null;
    return null;
  }

  private beginSingle(
    id: number,
    point: GalleryPoint,
    time: number,
    kind: GalleryPointerKind
  ) {
    this.single = {
      id,
      start: { ...point },
      last: { ...point },
      time,
      maxDistance: 0,
      startedZoomed: this.scale > 1.01,
      kind,
    };
  }

  private beginPinch() {
    this.swipeX = 0;
    const pair = Array.from(this.contacts.entries()).slice(0, 2);
    if (pair.length !== 2) return;
    const first = pair[0],
      second = pair[1];
    this.pinch = {
      ids: [first[0], second[0]],
      distance: Math.max(1, distance(first[1], second[1])),
      center: this.centered(midpoint(first[1], second[1])),
      scale: this.scale,
      x: this.x,
      y: this.y,
    };
    this.single = null;
  }

  private centered(point: GalleryPoint) {
    return { x: point.x - this.width / 2, y: point.y - this.height / 2 };
  }
  private validPoint(point: GalleryPoint) {
    return Number.isFinite(point.x) && Number.isFinite(point.y);
  }
  private insideImage(point: GalleryPoint) {
    const image = this.transform;
    return (
      this.validPoint(point) &&
      image.width > 0 &&
      point.x >= image.left &&
      point.y >= image.top &&
      point.x <= image.left + image.width * image.scale &&
      point.y <= image.top + image.height * image.scale
    );
  }
  private bound() {
    const fitted = this.fitted;
    const limitX = Math.max(0, (fitted.width * this.scale - this.width) / 2);
    const limitY = Math.max(0, (fitted.height * this.scale - this.height) / 2);
    this.x = clamp(this.x, -limitX, limitX);
    this.y = clamp(this.y, -limitY, limitY);
    if (this.scale === 1) {
      this.x = 0;
      this.y = 0;
    }
  }
}
