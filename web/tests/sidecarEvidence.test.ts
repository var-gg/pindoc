import { splitEvidenceEdges } from "../src/reader/sidecarEvidence";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testEvidenceEdgesDoNotRenderAsRegularReferences(): void {
  const out = splitEvidenceEdges(
    [
      { relation: "references", artifact_id: "a", slug: "regular" },
      { relation: "evidence", artifact_id: "b", slug: "proof" },
    ],
    [
      { relation: "Evidence", artifact_id: "c", slug: "incoming-proof" },
      { relation: "blocks", artifact_id: "d", slug: "blocked" },
    ],
  );

  assertEqual(out.regularRelates.length, 1, "regular outgoing count");
  assertEqual(out.regularRelates[0]?.slug, "regular", "regular outgoing slug");
  assertEqual(out.evidenceRelates.length, 1, "evidence outgoing count");
  assertEqual(out.evidenceRelates[0]?.slug, "proof", "evidence outgoing slug");
  assertEqual(out.evidenceRelatedBy.length, 1, "evidence incoming count");
  assertEqual(out.evidenceRelatedBy[0]?.slug, "incoming-proof", "evidence incoming slug");
  assertEqual(out.regularRelatedBy.length, 1, "regular incoming count");
  assertEqual(out.regularRelatedBy[0]?.slug, "blocked", "regular incoming slug");
}

function testPinsKeepReferencePriorityWhenEvidenceDuplicatesTarget(): void {
  const out = splitEvidenceEdges(
    [{ relation: "evidence", artifact_id: "pin-target", slug: "same-support" }],
    [],
  );
  const pinReferences = [{ path: "internal/pindoc/mcp/tools/task_queue.go" }];

  assertEqual(pinReferences.length, 1, "pin references stay visible");
  assertEqual(out.regularRelates.length, 0, "evidence does not duplicate in regular relation list");
  assertEqual(out.evidenceRelates.length, 1, "evidence has its own sidecar group");
}

testEvidenceEdgesDoNotRenderAsRegularReferences();
testPinsKeepReferencePriorityWhenEvidenceDuplicatesTarget();
