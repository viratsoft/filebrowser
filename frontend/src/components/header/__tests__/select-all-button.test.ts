import { describe, expect, it, vi } from "vitest";
import { createSSRApp } from "vue";
import { renderToString } from "vue/server-renderer";
import { createPinia } from "pinia";
import { useFileStore } from "@/stores/file";
import { useLayoutStore } from "@/stores/layout";
import SelectAllButton from "../SelectAllButton.vue";

vi.mock("vue-i18n", () => ({
  useI18n: () => ({
    t: (key: string) =>
      key === "buttons.selectAll" ? "Select all" : "Deselect all",
  }),
}));

function setup(count = 3) {
  const app = createSSRApp(SelectAllButton);
  const pinia = createPinia();
  app.use(pinia);
  const files = useFileStore(pinia);
  const layout = useLayoutStore(pinia);
  files.isFiles = true;
  files.req = {
    isDir: true,
    items: Array.from({ length: count }, (_, index) => ({ index })),
  } as Resource;
  return { app, files, layout };
}

describe("select-all icon button", () => {
  it("renders a native accessible icon button without the overflow menu", async () => {
    const { app } = setup();
    const html = await renderToString(app);
    expect(html).toContain("<button");
    expect(html).toContain('type="button"');
    expect(html).toContain('aria-label="Select all"');
    expect(html).toContain('aria-pressed="false"');
    expect(html).toContain("select_all");
    expect(html).not.toContain("dropdown");
  });

  it("shows Deselect all when every item is selected", async () => {
    const { app, files } = setup();
    files.toggleAllSelection();
    const html = await renderToString(app);
    expect(html).toContain('aria-label="Deselect all"');
    expect(html).toContain('aria-pressed="true"');
    expect(html).toContain(">deselect</i>");
  });

  it("disables interaction while the listing is loading", async () => {
    const { app, layout } = setup();
    layout.loading = true;
    expect(await renderToString(app)).toMatch(/<button[^>]*\sdisabled(?:\s|>)/);
  });

  it("hides the control for empty folders and non-file routes", async () => {
    expect(await renderToString(setup(0).app)).not.toContain("<button");
    const { app, files } = setup();
    files.isFiles = false;
    expect(await renderToString(app)).not.toContain("<button");
  });
});
