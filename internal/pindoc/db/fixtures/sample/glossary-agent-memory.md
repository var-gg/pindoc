## Definition

Agent memory is the durable project context that an agent can retrieve in a future session without relying on the current chat transcript.

## Usage

In Pindoc, agent memory is stored as artifacts: Decisions for rules, Tasks for implementation contracts, Debug notes for investigations, and Flow artifacts for handoffs. The goal is not to write more documents, but to keep the next session from rediscovering the same facts.

## Example

When an operator decides that sample data must never include private roadmap details, that rule belongs in a Decision artifact. The sample fixture loader can then cite the rule through tests and generic content review.
