import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { I18nProvider } from "../src/i18n";
import { IdentitySetup } from "../src/onboarding/IdentitySetup";
import { CreateProjectPage, CreateProjectSuccess } from "../src/reader/CreateProjectPage";
import ko from "../src/i18n/ko.json";
import en from "../src/i18n/en.json";
import type { CreateProjectResp } from "../src/api/client";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function tFrom(copy: Record<string, string>) {
  return (key: string, ...args: Array<string | number>): string => {
    let value = copy[key] ?? key;
    for (const arg of args) {
      value = value.replace(/%[sd]/, String(arg));
    }
    return value;
  };
}

function renderIdentity(projectLang: "en" | "ko"): string {
  return renderToStaticMarkup(
    <I18nProvider projectLang={projectLang}>
      <MemoryRouter>
        <IdentitySetup />
      </MemoryRouter>
    </I18nProvider>,
  );
}

function testIdentitySetupLocalizesKoAndEn(): void {
  const korean = renderIdentity("ko");
  assert(korean.includes("사용자 정보 설정"), "KO identity page should render localized title");
  assert(korean.includes("표시 이름"), "KO identity page should render localized display-name label");
  assert(korean.includes("사용자 만들기"), "KO identity page should render localized submit button");
  for (const blocked of ["Set your identity", "Display name", "Create identity", "Welcome,"]) {
    assert(!korean.includes(blocked), `KO identity page should not expose raw English: ${blocked}`);
  }

  const english = renderIdentity("en");
  assert(english.includes("Set your identity"), "EN identity page should render English title");
  assert(english.includes("Display name"), "EN identity page should render English label");
}

function renderCreateProjectWelcome(projectLang: "en" | "ko"): string {
  return renderToStaticMarkup(
    <I18nProvider projectLang={projectLang}>
      <MemoryRouter initialEntries={["/projects/new?welcome=1"]}>
        <CreateProjectPage />
      </MemoryRouter>
    </I18nProvider>,
  );
}

function testCreateProjectWelcomeUsesSecondStep(): void {
  const korean = renderCreateProjectWelcome("ko");
  assert(korean.includes("2 / 3 단계"), "CreateProject welcome should be step 2 in KO");

  const english = renderCreateProjectWelcome("en");
  assert(english.includes("Step 2 / 3"), "CreateProject welcome should be step 2 in EN");
}

function testCreateProjectSuccessShowsThreeCopyTargets(): void {
  const result: CreateProjectResp = {
    project_id: "project-id",
    slug: "shop-fe",
    name: "Shop Frontend",
    primary_language: "en",
    url: "/p/shop-fe/today",
    default_area: "misc",
    areas_created: 24,
    templates_created: 4,
    mcp_connect: {
      url: "http://127.0.0.1:5832/mcp",
      mcp_json: "{\n  \"mcpServers\": {}\n}",
      agent_prompt: "Please register Pindoc for project_slug=\"shop-fe\".",
    },
  };
  const html = renderToStaticMarkup(
    <MemoryRouter>
      <CreateProjectSuccess
        result={result}
        copied={null}
        onCopy={() => undefined}
        onCreateAnother={() => undefined}
        t={tFrom(en)}
      />
    </MemoryRouter>,
  );

  assert(html.includes("Step 3 / 3"), "CreateProject success should render final-step label");
  assert(html.includes("MCP URL only"), "CreateProject success should show URL copy target");
  assert(html.includes(".mcp.json snippet"), "CreateProject success should show JSON copy target");
  assert(html.includes("Agent prompt"), "CreateProject success should show agent prompt copy target");
  assert(html.includes("http://127.0.0.1:5832/mcp"), "CreateProject success should use BE-provided MCP URL");
}

function testNewProjectKoreanCopyUsesLocalizedStepLabels(): void {
  assert(ko["new_project.welcome.step"].includes("2 / 3"), "KO new-project welcome should be step 2");
  assert(ko["new_project.success.step"].includes("3 / 3"), "KO new-project success should be step 3");
  assert(!ko["new_project.welcome.subtitle"].includes("copy target"), "KO new-project subtitle should not expose raw English");
}

testIdentitySetupLocalizesKoAndEn();
testCreateProjectWelcomeUsesSecondStep();
testCreateProjectSuccessShowsThreeCopyTargets();
testNewProjectKoreanCopyUsesLocalizedStepLabels();
