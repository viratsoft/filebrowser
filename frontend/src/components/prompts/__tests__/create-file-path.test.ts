import { createSSRApp } from "vue";
import { renderToString } from "vue/server-renderer";
import { expect, it, vi } from "vitest";
import CreateFilePath from "../CreateFilePath.vue";

const { route, fileStore } = vi.hoisted(() => ({
  route: { path: "/files/Parent%20Folder/Rates%2520/" },
  fileStore: { isFiles: true, isListing: true },
}));

vi.mock("vue-router", () => ({ useRoute: () => route }));
vi.mock("@/stores/file", () => ({ useFileStore: () => fileStore }));

it("decodes existing folders once and displays the new name verbatim", async () => {
  const app = createSSRApp(CreateFilePath, { name: "New%20name", isDir: true });
  const html = await renderToString(app);

  expect(html).toContain("Parent Folder");
  expect(html).toContain("Rates%20");
  expect(html).toContain("New%20name");
});

it("handles an explicit destination with Unicode and a bare percent sign", async () => {
  const app = createSSRApp(CreateFilePath, {
    name: "New file.txt",
    path: "/files/%E0%A4%AB%E0%A5%8B%E0%A4%B2%E0%A5%8D%E0%A4%A1%E0%A4%B0/100%/",
  });
  const html = await renderToString(app);

  expect(html).toContain("फोल्डर");
  expect(html).toContain("100%");
  expect(html).toContain("New file.txt");
});
