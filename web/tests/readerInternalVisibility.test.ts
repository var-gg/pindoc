import { sidebarAgentRows } from "../src/reader/readerInternalVisibility";
import type { Aggregate } from "../src/reader/useReaderData";

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual<T>(actual: T, expected: T, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testDefaultAgentRowsHideRawInternalIds(): void {
  const agents: Aggregate[] = [
    { key: "codex", count: 4 },
    { key: "agent:codex", count: 8 },
    { key: "ag_7f3a2b1c", count: 2 },
    { key: "pindoc-reconcile-sweeper", count: 1 },
    { key: "system", count: 3 },
  ];
  const rows = sidebarAgentRows(agents, false);
  const visibleLabels = rows.map((row) => row.labelKey ?? row.key).join("|");

  assert(!visibleLabels.includes("ag_7f3a2b1c"), "generated agent id must not be visible by default");
  assert(!visibleLabels.includes("pindoc-reconcile-sweeper"), "sweeper id must not be visible by default");
  assert(!visibleLabels.includes("system|"), "raw system id must not be visible by default");
  assert(!visibleLabels.includes("agent:codex"), "agent-prefixed author id must not be visible");
  assert(visibleLabels.includes("codex"), "human-readable agent remains visible");
  assert(visibleLabels.includes("sidebar.agent_generated"), "generated agents are grouped");
  assert(visibleLabels.includes("sidebar.agent_system"), "system actors are separated");

  const codex = rows.find((row) => row.key === "codex");
  const generated = rows.find((row) => row.kind === "generated");
  const system = rows.find((row) => row.kind === "system");
  assertEqual(codex?.count, 12, "agent-prefixed author count merges into visible author");
  assertEqual(generated?.count, 2, "generated group count");
  assertEqual(system?.count, 4, "system group count");
}

function testDebugRowsExposeRawIds(): void {
  const rows = sidebarAgentRows([
    { key: "codex", count: 4 },
    { key: "ag_7f3a2b1c", count: 2 },
    { key: "pindoc-reconcile-sweeper", count: 1 },
  ], true);
  const keys = rows.map((row) => row.key).join("|");
  assert(keys.includes("ag_7f3a2b1c"), "debug mode shows generated agent id");
  assert(keys.includes("pindoc-reconcile-sweeper"), "debug mode shows sweeper id");
  assert(!rows.some((row) => row.labelKey), "debug mode does not replace rows with group labels");
}

testDefaultAgentRowsHideRawInternalIds();
testDebugRowsExposeRawIds();
