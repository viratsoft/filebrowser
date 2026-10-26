import { beforeEach, describe, expect, it, vi } from "vitest";
import Info from "../Info.vue";
import { files as api } from "@/api";

vi.mock("@/api", () => ({ files: { folderDetails: vi.fn() } }));
vi.mock("@/stores/file", () => ({ useFileStore: vi.fn() }));
vi.mock("@/stores/layout", () => ({ useLayoutStore: vi.fn() }));
vi.mock("@/utils", () => ({ filesize: (size: number) => `${size} bytes` }));

function context() {
  return {
    ...(Info as any).data(),
    folderPath: "/files/current/",
    $showError: vi.fn(),
  };
}
const load = (value: ReturnType<typeof context>) =>
  (Info as any).methods.loadFolderSize.call(value);

describe("on-demand folder details", () => {
  beforeEach(() => vi.clearAllMocks());

  it("does not request data when the dialog initializes", () => {
    const value = context();
    expect(value.folderDetails).toBeNull();
    expect(value.loadingFolderSize).toBe(false);
    expect(api.folderDetails).not.toHaveBeenCalled();
  });

  it("loads and refreshes size and recursive file count together", async () => {
    vi.mocked(api.folderDetails)
      .mockResolvedValueOnce({ size: 1024, numFiles: 3 })
      .mockResolvedValueOnce({ size: 2048, numFiles: 4 });
    const value = context();
    await load(value);
    expect(api.folderDetails).toHaveBeenCalledWith(
      "/files/current/",
      expect.any(AbortSignal)
    );
    expect(value.folderDetails).toEqual({ size: 1024, numFiles: 3 });
    await load(value);
    expect(value.folderDetails).toEqual({ size: 2048, numFiles: 4 });
    expect(value.loadingFolderSize).toBe(false);
  });

  it("prevents duplicate clicks while loading", async () => {
    let finish!: (value: { size: number; numFiles: number }) => void;
    vi.mocked(api.folderDetails).mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      })
    );
    const value = context();
    const pending = load(value);
    expect(value.loadingFolderSize).toBe(true);
    await load(value);
    expect(api.folderDetails).toHaveBeenCalledTimes(1);
    finish({ size: 0, numFiles: 0 });
    await pending;
    expect(value.folderDetails).toEqual({ size: 0, numFiles: 0 });
  });

  it("preserves the incomplete flag and clears it after a successful refresh", async () => {
    vi.mocked(api.folderDetails)
      .mockResolvedValueOnce({ size: 100, numFiles: 2, incomplete: true })
      .mockResolvedValueOnce({ size: 300, numFiles: 5 });
    const value = context();
    await load(value);
    expect(value.folderDetails.incomplete).toBe(true);
    expect(value.$showError).not.toHaveBeenCalled();
    await load(value);
    expect(value.folderDetails).toEqual({ size: 300, numFiles: 5 });
    expect(value.folderDetails.incomplete).toBeUndefined();
  });

  it("ignores stale results after the folder changes", async () => {
    let finish!: (value: { size: number; numFiles: number }) => void;
    vi.mocked(api.folderDetails).mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      })
    );
    const value = context();
    const pending = load(value);
    const signal = vi.mocked(api.folderDetails).mock.calls[0][1]!;
    value.folderPath = "/files/other/";
    (Info as any).watch.folderPath.call(value);
    expect(signal.aborted).toBe(true);
    finish({ size: 100, numFiles: 10 });
    await pending;
    expect(value.folderDetails).toBeNull();
    expect(value.loadingFolderSize).toBe(false);
  });

  it("does not update a closed dialog", async () => {
    let finish!: (value: { size: number; numFiles: number }) => void;
    vi.mocked(api.folderDetails).mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      })
    );
    const value = context();
    const pending = load(value);
    const signal = vi.mocked(api.folderDetails).mock.calls[0][1]!;
    (Info as any).beforeUnmount.call(value);
    expect(signal.aborted).toBe(true);
    finish({ size: 100, numFiles: 10 });
    await pending;
    expect(value.folderDetails).toBeNull();
  });

  it("keeps the previous result and allows retry after a refresh error", async () => {
    const failure = new Error("No connection");
    vi.mocked(api.folderDetails).mockRejectedValue(failure);
    const value = context();
    value.folderDetails = { size: 50, numFiles: 2 };
    await load(value);
    expect(value.folderDetails).toEqual({ size: 50, numFiles: 2 });
    expect(value.$showError).toHaveBeenCalledWith(failure);
    expect(value.loadingFolderSize).toBe(false);
  });

  it("targets the selected folder, and excludes files or multiple selections", () => {
    const folderPath = (Info as any).computed.folderPath;
    const value = {
      dir: true,
      selectedCount: 1,
      selected: [0],
      req: { items: [{ url: "/files/current/selected/" }] },
      $route: { path: "/files/current/" },
    };
    expect(folderPath.call(value)).toBe("/files/current/selected/");
    expect(folderPath.call({ ...value, selectedCount: 0 })).toBe(
      "/files/current/"
    );
    expect(folderPath.call({ ...value, selectedCount: 2 })).toBeNull();
    expect(folderPath.call({ ...value, dir: false })).toBeNull();
  });
});
