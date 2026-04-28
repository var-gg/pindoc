import { canShowTelemetryNav, telemetryDebugEnabled } from "../src/reader/opsAccess";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
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

testTelemetryNavVisibilityByRole();
testTelemetryNavDebugOverride();
