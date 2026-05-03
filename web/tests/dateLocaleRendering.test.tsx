import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import type { Artifact, ArtifactReadState, ChangeGroup } from "../src/api/client";
import { I18nProvider } from "../src/i18n";
import { Sidecar } from "../src/reader/Sidecar";
import { PindocTooltipProvider } from "../src/reader/Tooltip";
import { ChangeGroupCard } from "../src/reader/Today";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

const group: ChangeGroup = {
  group_id: "chg-date-locale",
  group_kind: "human_trigger",
  grouping_key: { kind: "manual", value: "date-locale", confidence: "high" },
  commit_summary: "Date locale regression fixture",
  revision_count: 2,
  artifact_count: 1,
  first_artifact: {
    id: "artifact-1",
    slug: "date-locale-task",
    title: "Date locale task",
    type: "Task",
    area_slug: "ui",
  },
  areas: ["ui"],
  authors: ["agent:codex"],
  time_start: "2026-05-03T08:00:00",
  time_end: "2026-05-03T08:39:39",
  importance: { score: 2, level: "high" },
  verification_state: "partially_verified",
};

const readState: ArtifactReadState = {
  artifact_id: "artifact-1",
  read_state: "read",
  completion_pct: 100,
  last_seen_at: "2026-05-03T09:14:35",
  event_count: 1,
};

const detail: Artifact = {
  id: "artifact-1",
  slug: "date-locale-task",
  type: "Task",
  title: "Date locale task",
  area_slug: "ui",
  visibility: "org",
  completeness: "partial",
  status: "published",
  review_state: "auto_published",
  author_id: "agent:codex",
  published_at: "2026-05-03T08:39:39",
  updated_at: "2026-05-03T09:14:35",
  task_meta: {
    status: "open",
    priority: "p2",
    assignee: "agent:codex",
    due_at: "2026-05-04T10:00:00",
  },
  artifact_meta: {},
  body_markdown: "## Purpose\n\nEnglish locale date rendering regression fixture.\n\n## TODO\n\n- [ ] Date output uses UI locale.",
  tags: ["qa"],
  created_at: "2026-05-03T08:39:39",
  relates_to: [],
  related_by: [],
  pins: [],
};

function renderEnglishTodayAndSidecar(): string {
  return renderToStaticMarkup(
    <I18nProvider projectLang="en">
      <PindocTooltipProvider>
        <MemoryRouter>
          <ChangeGroupCard
            group={group}
            projectSlug="pindoc"
            orgSlug="default"
            areaNameBySlug={new Map([["ui", "UI"]])}
            onSelectArea={() => undefined}
            selectedArtifactSlug={null}
            onSelectArtifact={() => undefined}
            readState={readState}
          />
          <Sidecar
            projectSlug="pindoc"
            orgSlug="default"
            detail={detail}
          />
        </MemoryRouter>
      </PindocTooltipProvider>
    </I18nProvider>,
  );
}

function testEnglishRenderingDoesNotUseKoreanMeridiem(): void {
  const html = renderEnglishTodayAndSidecar();
  assert(!html.includes("오전"), "EN Today + Sidecar render should not include Korean AM marker");
  assert(!html.includes("오후"), "EN Today + Sidecar render should not include Korean PM marker");
}

testEnglishRenderingDoesNotUseKoreanMeridiem();
