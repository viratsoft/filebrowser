import { createPinia } from "pinia";
import { expect, it } from "vitest";
import { useLayoutStore } from "../layout";

it.each([
  ["sidebar", "newDir"],
  ["sidebar", "newFile"],
  ["more", "info"],
])("dismisses %s when opening %s", (menu, prompt) => {
  const layout = useLayoutStore(createPinia());
  layout.showHover(menu);
  layout.showHover({ prompt });
  expect(layout.currentPromptName).toBe(prompt);
  layout.closeHovers();
  expect(layout.currentPromptName).toBeUndefined();
});

it("restores the parent dialog after closing a nested dialog", () => {
  const layout = useLayoutStore(createPinia());
  layout.showHover("move");
  layout.showHover("newDir");
  layout.closeHovers();
  expect(layout.currentPromptName).toBe("move");
});
