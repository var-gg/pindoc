## 증상 (Symptom)

A fictional import job times out after thirty seconds while processing a small markdown bundle. The Reader still responds, but the import result never appears.

## 재현 (Reproduction)

- [x] Start a local fixture import.
- [x] Submit a bundle with six small markdown documents.
- [x] Observe the timeout in the import log.

## 가설 (Hypotheses tried)

The first hypothesis was that the markdown parser was blocking on frontmatter. A small single-file import completed, so the parser was not the main bottleneck.

The second hypothesis was that all files were embedded in one transaction with unnecessary repeated lookups. Grouping area lookups once per run removed the timeout in this fictional example.

## 원인 (Root cause)

The sample root cause is repeated per-file setup work inside the import loop. It is written as a narrative so future readers can see why the implementation changed.

## 해결 (Resolution)

Cache static lookup data once at the start of the import and reuse it for each fixture artifact. Keep the import idempotent so retrying after a timeout does not create duplicates.

## 검증 (Verification)

- [x] Run the fixture import twice and confirm the artifact count remains stable.
- [x] Confirm every inserted artifact has revision 1.
