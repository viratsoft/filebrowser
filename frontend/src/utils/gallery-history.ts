import type { Router } from "vue-router";

export async function closeGalleryRoute(router: Router, folder: string) {
  const back = router.options.history.state.back;
  if (
    typeof back === "string" &&
    router.resolve(back).path.replace(/\/+$/, "") === folder.replace(/\/+$/, "")
  ) {
    router.back();
    return;
  }
  await router.replace({ path: folder });
}
