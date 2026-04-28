import { paletteOpenAfterProjectSwitcherToggle, projectSwitcherOpenAfterPaletteChange } from "../src/reader/overlayStack";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testPaletteOpenClosesProjectSwitcher(): void {
  assertEqual(projectSwitcherOpenAfterPaletteChange(true, true), false, "opening CmdK closes project switcher");
  assertEqual(projectSwitcherOpenAfterPaletteChange(true, false), true, "closed CmdK leaves project switcher state alone");
}

function testProjectSwitcherOpenClosesPalette(): void {
  assertEqual(paletteOpenAfterProjectSwitcherToggle(true, true), false, "opening project switcher closes CmdK");
  assertEqual(paletteOpenAfterProjectSwitcherToggle(false, true), true, "closing project switcher leaves CmdK state alone");
}

testPaletteOpenClosesProjectSwitcher();
testProjectSwitcherOpenClosesPalette();
