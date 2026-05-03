import { canShowTelemetryNav, telemetryDebugEnabled } from "../src/reader/opsAccess";
import {
  filterTelemetryRecentByTool,
  telemetrySearchForSelectedTool,
  telemetrySelectedToolFromSearch,
  toggleTelemetryToolSelection,
} from "../src/ops/telemetryViewModel";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function assertDeepEqual(actual: unknown, expected: unknown, message: string): void {
  const actualJson = JSON.stringify(actual);
  const expectedJson = JSON.stringify(expected);
  if (actualJson !== expectedJson) {
    throw new Error(`${message}: got ${actualJson}, want ${expectedJson}`);
  }
}

function testTelemetryNavVisibilityByRole(): void {
  assertEqual(canShowTelemetryNav("owner", false), true, "owner sees telemetry nav");
  assertEqual(canShowTelemetryNav("editor", false), false, "editor does not see telemetry nav by default");
  assertEqual(canShowTelemetryNav("viewer", false), false, "viewer does not see telemetry nav by default");
  assertEqual(canShowTelemetryNav(undefined, false), false, "anonymous/default role does not see telemetry nav");
}

function testTelemetryNavDebugOverride(): void {
  assertEqual(telemetryDebugEnabled("?ops=1", null), true, "ops query enables debug nav");
  assertEqual(telemetryDebugEnabled("?debug=ops", null), true, "debug query enables debug nav");
  assertEqual(telemetryDebugEnabled("", "1"), true, "local debug flag enables debug nav");
  assertEqual(canShowTelemetryNav("viewer", telemetryDebugEnabled("?ops=1", null)), true, "debug override shows telemetry nav");
}

function testTelemetryToolSelectionModel(): void {
  assertEqual(telemetrySelectedToolFromSearch("?tool=pindoc.task.queue"), "pindoc.task.queue", "tool query is selected");
  assertEqual(telemetrySearchForSelectedTool("?window=24h", "pindoc.task.queue"), "?window=24h&tool=pindoc.task.queue", "tool query is written");
  assertEqual(telemetrySearchForSelectedTool("?window=24h&tool=pindoc.task.queue", ""), "?window=24h", "tool query is cleared");
  assertEqual(toggleTelemetryToolSelection("pindoc.task.queue", "pindoc.task.queue"), "", "same tool click clears selection");
  assertEqual(toggleTelemetryToolSelection("", "pindoc.task.queue"), "pindoc.task.queue", "new tool click selects tool");

  assertDeepEqual(
    filterTelemetryRecentByTool([
      { tool_name: "pindoc.task.queue", started_at: "2026-05-03T00:00:00Z" },
      { tool_name: "pindoc.context_for_task", started_at: "2026-05-03T00:01:00Z" },
    ], "pindoc.task.queue"),
    [{ tool_name: "pindoc.task.queue", started_at: "2026-05-03T00:00:00Z" }],
    "recent calls are filtered by selected tool",
  );
}

testTelemetryNavVisibilityByRole();
testTelemetryNavDebugOverride();
testTelemetryToolSelectionModel();
