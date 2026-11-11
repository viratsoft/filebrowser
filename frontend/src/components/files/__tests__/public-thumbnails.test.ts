import { beforeEach, describe, expect, it, vi } from "vitest";
import { createSSRApp } from "vue";
import { renderToString } from "vue/server-renderer";
import { createPinia } from "pinia";
import ListingItem from "../ListingItem.vue";
import { files as api } from "@/api";

vi.mock("@/stores/auth", () => ({ useAuthStore: () => ({ user: null }) }));
vi.mock("@/utils/constants", () => ({ enableThumbs: true }));
vi.mock("@/utils", () => ({ filesize: (size: number) => `${size} bytes` }));
vi.mock("@/utils/upload", () => ({}));
vi.mock("@/api", () => ({
  files: { getPreviewURL: vi.fn(() => "/api/preview/thumb/photo.png") },
}));
vi.mock("dayjs", () => ({ default: () => ({ fromNow: () => "just now" }) }));
vi.mock("vue-router", () => ({ useRouter: () => ({ push: vi.fn() }) }));

async function render(props: {
  readOnly: boolean;
  type?: string;
  publicThumbnailUrl?: string;
}) {
  const app = createSSRApp(ListingItem, {
    name: "photo.png",
    isDir: false,
    url: "/share/hash/photo.png",
    path: "/photo.png",
    type: "image",
    size: 20,
    modified: "2026-11-11T00:00:00Z",
    index: 0,
    ...props,
  });
  app.use(createPinia());
  app.provide("$showError", vi.fn());
  return renderToString(app);
}

describe("readonly shared file thumbnails", () => {
  beforeEach(() => vi.clearAllMocks());

  it("renders a public lazy thumbnail without calling the authenticated preview API", async () => {
    const html = await render({
      readOnly: true,
      publicThumbnailUrl: "/api/public/thumb/hash/photo.png?token=share-token",
    });
    expect(html).toContain(
      'src="/api/public/thumb/hash/photo.png?token=share-token"'
    );
    expect(html).toContain('loading="lazy"');
    expect(html).toContain('referrerpolicy="no-referrer"');
    expect(html).toContain('draggable="false"');
    expect(api.getPreviewURL).not.toHaveBeenCalled();
  });

  it("uses a file icon when shared thumbnails are disabled or absent", async () => {
    expect(await render({ readOnly: true })).not.toContain("<img");
    expect(api.getPreviewURL).not.toHaveBeenCalled();
  });

  it("never treats a non-image as a thumbnail", async () => {
    expect(
      await render({
        readOnly: true,
        type: "text",
        publicThumbnailUrl: "/api/public/thumb/hash/file.txt",
      })
    ).not.toContain("<img");
  });

  it("preserves the authenticated user's original thumbnail path", async () => {
    const html = await render({
      readOnly: false,
      publicThumbnailUrl: "/api/public/thumb/hash/photo.png",
    });
    expect(api.getPreviewURL).toHaveBeenCalled();
    expect(html).toContain('src="/api/preview/thumb/photo.png"');
    expect(html).toContain('loading="lazy"');
    expect(html).toContain('referrerpolicy="no-referrer"');
    expect(html).not.toContain('src="/api/public/thumb/');
  });
});
