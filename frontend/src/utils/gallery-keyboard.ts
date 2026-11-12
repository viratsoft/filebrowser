export function consumeGallerySelectionShortcut(event: KeyboardEvent) {
  if (
    event.key !== " " ||
    event.defaultPrevented ||
    event.altKey ||
    event.ctrlKey ||
    event.metaKey ||
    event.shiftKey ||
    event
      .composedPath()
      .some(
        (target) =>
          target instanceof HTMLElement &&
          (target.isContentEditable ||
            target.matches(
              "button, a[href], input, select, textarea, [role=button], [role=switch]"
            ))
      )
  ) {
    return false;
  }

  event.preventDefault();
  return !event.repeat;
}
