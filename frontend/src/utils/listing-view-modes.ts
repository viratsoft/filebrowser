export const listingViewModes: Record<
  ViewModeType,
  { icon: string; next: ViewModeType }
> = {
  list: { icon: "view_module", next: "mosaic" },
  mosaic: { icon: "grid_view", next: "mosaic gallery" },
  "mosaic gallery": { icon: "view_list", next: "list" },
};
