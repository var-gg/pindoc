import ko = require("../src/i18n/ko.json");
import en = require("../src/i18n/en.json");

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
  ].join("\n");

  for (const blocked of ["Area", "Type", "Agent", "Briefing", "Surface", "Inspector", "artifact", "change", "trusted local", "immutable", "surface 도움말", "Role", "export"]) {
    assert(!snapshot.includes(blocked), `KO chrome snapshot should hide raw English token: ${blocked}`);
  }
  assert(!snapshot.includes("(미확인)"), "unknown byline should use neutral copy");
  assert(!snapshot.includes("완료 주장"), "Task completion copy should hide internal claimed_done wording");
  assert(!snapshot.includes("surface.name.surface"), "surface fallback should never expose raw i18n keys");
  assertEqual(ko["reader.byline_unknown"], "작성자 정보 없음", "unknown byline should be explicit and neutral");
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

testKoreanReaderMetaCopySnapshot();
testKoreanReaderChromeCopyHidesRawEnglishLabels();
testEnglishNavLabelsUseTitleCase();
