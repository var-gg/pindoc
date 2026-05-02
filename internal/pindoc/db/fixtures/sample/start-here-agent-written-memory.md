## TL;DR

Pindoc stores durable project memory as artifacts rather than burying it in chat history.

## Purpose

This sample project shows the core Pindoc loop: decisions capture durable rules, tasks carry acceptance criteria, debug notes preserve evidence, and handoffs make the next agent session cheaper to start.

The content is intentionally generic. It is safe to ship in the OSS repository and should not be confused with the operator's own project data.

## What To Read First

Start with the Decision sample to see how project rules are recorded. Then open the Task sample to see what an agent can implement, verify, and close. The Debug and Flow samples show how investigation and handoff notes keep context from disappearing between sessions.

## Re-Verification

On a fresh install with `PINDOC_WITH_SAMPLE=true`, the project switcher should show `pindoc-tour` and this artifact should be visible in the Wiki list.

## Related

These sample artifacts are fixtures for first-run onboarding. Delete the `pindoc-tour` project when the real project has enough memory of its own.
