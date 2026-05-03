import { renderToStaticMarkup } from "react-dom/server";
import { ProjectSettingsVisibilityPanel } from "../src/reader/ProjectSettingsVisibilityPanel";

const copy: Record<string, string> = {
  "artifact.visibility.public": "public",
  "artifact.visibility.org": "org",
  "artifact.visibility.private": "private",
  "settings.owner_only": "Only project owners can change this setting.",
  "settings.project_visibility_section_label": "Project visibility",
  "settings.project_visibility_section_desc": "Control who can discover this project and the default tier for new artifacts.",
  "settings.project_visibility_label": "Project access",
  "settings.project_visibility_public_desc": "Visible to anyone.",
  "settings.project_visibility_org_desc": "Visible to members.",
  "settings.project_visibility_private_desc": "Visible to owners.",
  "settings.visibility_label": "Default artifact visibility",
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
      projectVisibility="org"
      defaultVisibility="org"
      saving={null}
      t={t}
      onProjectVisibilityChange={() => undefined}
      onDefaultVisibilityChange={() => undefined}
    />,
  );

  assert(count(html, 'type="radio"') === 6, "owner render should include six radio controls");
  assert(!html.includes("disabled"), "owner radio controls should be enabled");
  assert(html.includes("Project access"), "project visibility label should render");
  assert(html.includes("Default artifact visibility"), "default visibility label should render");
  assert(html.includes("Readable by anyone."), "public description should render");
  assert(html.includes("Readable by members."), "org description should render");
  assert(html.includes("Readable by owners."), "private description should render");
}

function testNonOwnerVisibilityRadiosRenderReadOnly(): void {
  const html = renderToStaticMarkup(
    <ProjectSettingsVisibilityPanel
      canEdit={false}
      projectVisibility="private"
      defaultVisibility="private"
      saving={null}
      t={t}
      onProjectVisibilityChange={() => undefined}
      onDefaultVisibilityChange={() => undefined}
    />,
  );

  assert(count(html, 'type="radio"') === 6, "non-owner render should include six radio controls");
  assert(count(html, 'disabled=""') === 6, "non-owner radio controls should be disabled");
  assert(
    html.includes("Only project owners can change this setting."),
    "non-owner render should include owner-only warning",
  );
}

testOwnerVisibilityRadiosRenderEnabled();
testNonOwnerVisibilityRadiosRenderReadOnly();
