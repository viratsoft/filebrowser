export type ShareSortField = "name" | "size" | "modified";

export interface ShareSort {
  by: ShareSortField;
  asc: boolean;
}

const byName = new Intl.Collator(undefined, {
  numeric: true,
  sensitivity: "base",
});

export function sortShareItems(items: ResourceItem[], sort: ShareSort) {
  const direction = sort.asc ? 1 : -1;
  return items
    .map((item) => ({
      item,
      value:
        sort.by === "modified"
          ? Date.parse(item.modified) || 0
          : sort.by === "size"
            ? item.size
            : 0,
    }))
    .sort((a, b) => {
      if (a.item.isDir !== b.item.isDir) return a.item.isDir ? -1 : 1;
      return (
        direction *
        (a.value - b.value || byName.compare(a.item.name, b.item.name))
      );
    })
    .map(({ item }, index) => ({ ...item, index }));
}
