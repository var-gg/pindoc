## TL;DR

Project rules should be recorded as Decision artifacts so future agents can retrieve them before editing.

## Context

Agent sessions are short-lived and chat history is not a stable source of truth. If an operator decides a rule once, the next session needs a durable place to find it without rereading a long conversation.

## Decision

Record durable project rules as Decision artifacts, then let implementation Tasks link to those Decisions when they carry out the rule.

## Rationale

Decision artifacts create a small, searchable contract. They are easier to cite than a chat transcript and easier to supersede than comments spread across unrelated files.

## Alternatives considered

Keeping rules only in README files is simple, but it mixes user-facing setup instructions with operational memory that mostly helps agents.

Keeping rules only in chat is fastest at the moment of discussion, but it makes later sessions repeat discovery work and increases the chance of drift.

## Consequences

The project gains a small writing cost whenever a durable rule changes. In exchange, future agents can retrieve the rule through context search and can attach implementation evidence to the relevant Task.
