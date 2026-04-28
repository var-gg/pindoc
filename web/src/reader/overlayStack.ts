export function projectSwitcherOpenAfterPaletteChange(
  projectSwitcherOpen: boolean,
  paletteOpen: boolean,
): boolean {
  return paletteOpen ? false : projectSwitcherOpen;
}

export function paletteOpenAfterProjectSwitcherToggle(
  nextProjectSwitcherOpen: boolean,
  paletteOpen: boolean,
): boolean {
  return nextProjectSwitcherOpen ? false : paletteOpen;
}
