// Agent-avatar styling. Matches the design bundle's six-color rotation for
// authoring agents. Keeping the mapping here rather than in CSS so it can
// also drive agent list rendering in the sidebar.

export type AgentAvatar = { initials: string; className: string };

const knownAgents: Record<string, AgentAvatar> = {
  "claude-code": { initials: "cc", className: "av" },
  cursor:        { initials: "cr", className: "av cr" },
  codex:         { initials: "cx", className: "av cx" },
  "pindoc-seed": { initials: "ps", className: "av" },
  system:        { initials: "sy", className: "av" },
};

export function agentAvatar(authorId: string): AgentAvatar {
  const known = knownAgents[authorId];
  if (known) return known;
  // Unknown agent: two-letter mono initials from first two alnum chars.
  const clean = authorId.replace(/[^a-z0-9]/gi, "");
  const initials = (clean.slice(0, 2) || "??").toLowerCase();
  return { initials, className: "av" };
}
