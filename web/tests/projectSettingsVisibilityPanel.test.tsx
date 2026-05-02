import { renderToStaticMarkup } from "react-dom/server";
import { ProjectSettingsVisibilityPanel } from "../src/reader/ProjectSettingsVisibilityPanel";

const copy: Record<string, string> = {
  "artifact.visibility.public": "public",
  "artifact.visibility.org": "org",
  "artifact.visibility.private": "private",
  "settings.owner_only": "Only project owners can change this setting.",
  "settings.visibility_label": "Default visibility",
  "settings.visibility_public_desc": "Readable by anyone.",
  "settings.visibility_org_desc": "Readable by members.",
  "settings.visibility_private_desc": "Readable by owners.",
};

function t(key: string): string {
  return copy[key] ?? key;
}

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function count(haystack: string, needle: string): number {
  return haystack.split(needle).length - 1;
}

function testOwnerVisibilityRadiosRenderEnabled(): void {
  const html = renderToStaticMarkup(
    <ProjectSettingsVisibilityPanel
      canEdit={true}
      defaultVisibility="org"
      saving={false}
      t={t}
      onChange={() => undefined}
    />,
  );

  assert(count(html, 'type="radio"') === 3, "owner render should include three radio controls");
  assert(!html.includes("disabled"), "owner radio controls should be enabled");
  assert(html.includes("Default visibility"), "panel label should render");
  assert(html.includes("Readable by anyone."), "public description should render");
  assert(html.includes("Readable by members."), "org description should render");
  assert(html.includes("Readable by owners."), "private description should render");
}

function testNonOwnerVisibilityRadiosRenderReadOnly(): void {
  const html = renderToStaticMarkup(
    <ProjectSettingsVisibilityPanel
      canEdit={false}
      defaultVisibility="private"
      saving={false}
      t={t}
      onChange={() => undefined}
    />,
  );

  assert(count(html, 'type="radio"') === 3, "non-owner render should include three radio controls");
  assert(count(html, 'disabled=""') === 3, "non-owner radio controls should be disabled");
  assert(
    html.includes("Only project owners can change this setting."),
    "non-owner render should include owner-only warning",
  );
}

testOwnerVisibilityRadiosRenderEnabled();
testNonOwnerVisibilityRadiosRenderReadOnly();
