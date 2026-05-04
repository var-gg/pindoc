import ko from "../src/i18n/ko.json";
import en from "../src/i18n/en.json";
import {
  fieldForProjectCreateError,
  isProjectCreateSubmitDisabled,
  projectCreateErrorMessage,
  projectReservedSlugCategory,
  validateProjectSlugInput,
} from "../src/reader/projectSlugPolicy";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testKoreanReaderMetaCopySnapshot(): void {
  const snapshot = [
    ko["reader.toc_title"],
    ko["reader.read_state.unseen"],
    ko["reader.read_state.glanced"],
    ko["reader.read_state.read"],
    ko["reader.read_state.deeply_read"],
  ].join("\n");

  assertEqual(ko["reader.toc_title"], "이 문서", "TOC title should be natural Korean");
  assertEqual(ko["reader.read_state.glanced"], "훑어봄", "glanced label should use the corrected Korean copy");
  assert(!snapshot.includes("이 로서"), "TOC title snapshot must not include broken Korean");
  assert(!snapshot.includes("흩어봄"), "read-state snapshot must not include misspelled Korean");
}

function testKoreanReaderChromeCopyHidesRawEnglishLabels(): void {
  const snapshot = [
    ko["wiki.section_areas"],
    ko["sidebar.types"],
    ko["wiki.load_more"],
    ko["wiki.loading_more"],
    ko["wiki.load_more_error"],
    ko["sidebar.agents"],
    ko["sidebar.unread_count"],
    ko["nav.mobile_menu"],
    ko["nav.mobile_surfaces"],
    ko["profile.role"],
    ko["profile.auth_mode.trusted_local"],
    ko["help.open"],
    ko["help.title"],
    ko["help.surface_label"],
    ko["help.types_title"],
    ko["help.areas_title"],
    ko["help.docs_title"],
    ko["surface.header_label"],
    ko["surface.name.artifact"],
    ko["surface.name.inbox"],
    ko["surface.name.graph"],
    ko["today.eyebrow"],
    ko["today.open_area"],
    ko["today.export_project"],
    ko["today.export_area"],
    ko["today.filters"],
    ko["today.brief_bullet_no_groups"],
    ko["today.inspector_empty"],
    ko["reader.inspector_empty"],
    ko["tasks.inspector_empty"],
    ko["reader.byline_unknown"],
    ko["surface.name.surface"],
    ko["surface.name.history"],
    ko["surface.name.diff"],
    ko["history.title"],
    ko["surface.not_found"],
    ko["tasks.col_claimed_done"],
    ko["tasks.summary_review_hint"],
    ko["trust.task.claimed_done.label"],
    ko["trust.task.claimed_done.tip"],
    ko["today.type_distribution"],
    ko["today.kind_human_trigger"],
    ko["today.kind_auto_sync"],
    ko["today.kind_maintenance"],
    ko["today.kind_system"],
    ko["new_project.field.language"],
    ko["invite.modal.role"],
    ko["signup.meta.role"],
    ko["tasks.mobile_scroll_hint"],
    ko["ops.title"],
    ko["artifact.visibility.public"],
    ko["artifact.visibility.org"],
    ko["artifact.visibility.private"],
  ].join("\n");

  for (const blocked of ["Area", "Type", "Agent", "Briefing", "Surface", "Inspector", "artifact", "change", "trusted local", "immutable", "surface 도움말", "Role", "export"]) {
    assert(!snapshot.includes(blocked), `KO chrome snapshot should hide raw English token: ${blocked}`);
  }
  assert(!snapshot.includes("(미확인)"), "unknown byline should use neutral copy");
  assert(!snapshot.includes("완료 주장"), "Task completion copy should hide internal claimed_done wording");
  assert(!snapshot.includes("surface.name.surface"), "surface fallback should never expose raw i18n keys");
  assertEqual(ko["surface.name.history"], "수정 이력", "history surface label should be localized");
  assertEqual(ko["surface.name.diff"], "차이", "diff surface label should be localized");
  assertEqual(ko["history.title"], "수정 이력", "history title should not fall back to raw English");
  assertEqual(ko["reader.byline_unknown"], "작성자 정보 없음", "unknown byline should be explicit and neutral");
  assertEqual(ko["artifact.visibility.public"], "공개", "KO public visibility label should be localized");
  assertEqual(ko["artifact.visibility.org"], "조직", "KO org visibility label should be localized");
  assertEqual(ko["artifact.visibility.private"], "비공개", "KO private visibility label should be localized");
  assertEqual(ko["wiki.load_more"], "더 보기", "wiki pagination action should be localized");
}

function testEnglishNavLabelsUseTitleCase(): void {
  const expectedLabels: Array<[keyof typeof en, string]> = [
    ["nav.today", "Today"],
    ["nav.wiki_reader", "Wiki"],
    ["nav.tasks", "Tasks"],
    ["nav.graph", "Graph"],
    ["nav.inbox", "Inbox"],
    ["nav.settings", "Settings"],
  ];

  for (const [key, expected] of expectedLabels) {
    assertEqual(en[key], expected, `EN nav label ${key} should use Title Case`);
  }
}

function testCmdKCopyDoesNotAdvertiseMissingCommands(): void {
  assertEqual(ko["nav.search_hint"], "문서 또는 commit SHA 검색...", "KO nav search hint should match implemented Cmd-K scope");
  assertEqual(en["nav.search_hint"], "Search artifacts or a commit SHA...", "EN nav search hint should match implemented Cmd-K scope");
  assertEqual(ko["cmdk.placeholder"], "문서 또는 commit SHA 검색...", "KO Cmd-K placeholder should not advertise commands");
  assertEqual(en["cmdk.placeholder"], "Search artifacts or a commit SHA...", "EN Cmd-K placeholder should not advertise commands");
  assertEqual(ko["cmdk.hint"], "문서나 commit SHA를 검색하세요.", "KO Cmd-K hint should align with placeholder scope");
  assertEqual(en["cmdk.hint"], "Search artifacts or a commit SHA in this project.", "EN Cmd-K hint should align with placeholder scope");

  const snapshot = [
    ko["nav.search_hint"],
    en["nav.search_hint"],
    ko["cmdk.placeholder"],
    en["cmdk.placeholder"],
    ko["cmdk.hint"],
    en["cmdk.hint"],
  ].join("\n").toLowerCase();

  for (const blocked of ["명령어", "command"]) {
    assert(!snapshot.includes(blocked), `Cmd-K copy should not advertise missing commands: ${blocked}`);
  }
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

function testCreateProjectErrorsHideRawCodes(): void {
  const codes = [
    "SLUG_INVALID",
    "SLUG_RESERVED",
    "SLUG_TAKEN",
    "NAME_REQUIRED",
    "LANG_REQUIRED",
    "LANG_INVALID",
    "GIT_REMOTE_URL_INVALID",
    "BAD_JSON",
    "INTERNAL_ERROR",
    "UNKNOWN",
  ];

  for (const copy of [ko, en]) {
    const t = tFrom(copy as Record<string, string>);
    for (const code of codes) {
      const message = projectCreateErrorMessage(t, code);
      assert(!message.includes(`[${code}]`), `create-project message should not include bracketed code ${code}`);
      assert(!message.includes(code), `create-project message should not include raw code ${code}`);
    }
  }
}

function testCreateProjectErrorFieldMapping(): void {
  assertEqual(fieldForProjectCreateError("SLUG_INVALID"), "slug", "SLUG_INVALID should map to slug");
  assertEqual(fieldForProjectCreateError("SLUG_RESERVED"), "slug", "SLUG_RESERVED should map to slug");
  assertEqual(fieldForProjectCreateError("SLUG_TAKEN"), "slug", "SLUG_TAKEN should map to slug");
  assertEqual(fieldForProjectCreateError("NAME_REQUIRED"), "name", "NAME_REQUIRED should map to name");
  assertEqual(fieldForProjectCreateError("LANG_REQUIRED"), "language", "LANG_REQUIRED should map to language");
  assertEqual(fieldForProjectCreateError("LANG_INVALID"), "language", "LANG_INVALID should map to language");
  assertEqual(fieldForProjectCreateError("INTERNAL_ERROR"), null, "INTERNAL_ERROR should stay form-level");
}

function testCreateProjectSlugClientValidation(): void {
  for (const raw of ["테스트", "1abc", "Test", "a", "a".repeat(41), "foo_bar"]) {
    assertEqual(validateProjectSlugInput(raw), "SLUG_INVALID", `${raw} should fail shape validation`);
  }
  for (const raw of ["admin", "api", "wiki", "new"]) {
    assertEqual(validateProjectSlugInput(raw), "SLUG_RESERVED", `${raw} should fail reserved validation`);
  }
  assertEqual(validateProjectSlugInput("var-gg-test"), null, "valid project slug should pass client validation");
}

function testCreateProjectReservedSlugMessagesUseCategoryExamples(): void {
  const examples = [
    ["support", "service"],
    ["billing", "billing"],
    ["dashboard", "workspace"],
    ["signup", "auth"],
  ] as const;

  for (const [slug, category] of examples) {
    assertEqual(projectReservedSlugCategory(slug), category, `${slug} should map to ${category}`);
    for (const copy of [ko, en]) {
      const t = tFrom(copy as Record<string, string>);
      const message = projectCreateErrorMessage(t, "SLUG_RESERVED", { slug });
      assert(message.includes(`/${slug}`), `reserved slug message should include category example /${slug}`);
    }
  }
}

function testCreateProjectSubmitDisabledState(): void {
  assert(
    isProjectCreateSubmitDisabled({ slug: "", name: "Var GG", primaryLanguage: "ko", submitting: false }),
    "empty slug should disable submit",
  );
  assert(
    isProjectCreateSubmitDisabled({ slug: "var-gg", name: "", primaryLanguage: "ko", submitting: false }),
    "empty name should disable submit",
  );
  assert(
    isProjectCreateSubmitDisabled({ slug: "admin", name: "Admin", primaryLanguage: "ko", submitting: false }),
    "reserved slug should disable submit",
  );
  assert(
    isProjectCreateSubmitDisabled({ slug: "var-gg", name: "Var GG", primaryLanguage: "ja", submitting: false }),
    "unsupported JA web language should disable submit",
  );
  assert(
    isProjectCreateSubmitDisabled({ slug: "var-gg", name: "Var GG", primaryLanguage: "ko", submitting: true }),
    "submitting state should disable submit",
  );
  assert(
    !isProjectCreateSubmitDisabled({ slug: "var-gg", name: "Var GG", primaryLanguage: "ko", submitting: false }),
    "valid form should enable submit",
  );
}

testKoreanReaderMetaCopySnapshot();
testKoreanReaderChromeCopyHidesRawEnglishLabels();
testEnglishNavLabelsUseTitleCase();
testCmdKCopyDoesNotAdvertiseMissingCommands();
testCreateProjectErrorsHideRawCodes();
testCreateProjectErrorFieldMapping();
testCreateProjectSlugClientValidation();
testCreateProjectReservedSlugMessagesUseCategoryExamples();
testCreateProjectSubmitDisabledState();
