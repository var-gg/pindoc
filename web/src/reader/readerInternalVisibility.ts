import type { Aggregate } from "./useReaderData";

export type SidebarAgentRow = Aggregate & {
  kind: "agent" | "generated" | "system";
  labelKey?: string;
  avatarKey: string;
  rawKeys?: string[];
};

const GENERATED_AGENT_PATTERN = /^ag_[a-z0-9]+$/i;

export function isGeneratedAgentId(agentId: string): boolean {
  return GENERATED_AGENT_PATTERN.test(agentId.trim());
}

export function isInternalAgentId(agentId: string): boolean {
  const id = agentId.trim().toLowerCase();
  return id === "system" ||
    id === "system_auto" ||
    id.includes("sweeper") ||
    id.startsWith("pindoc-");
}

export function sidebarAgentRows(
  agents: Aggregate[],
  includeInternal: boolean,
): SidebarAgentRow[] {
  if (includeInternal) {
    return agents.map((agent) => ({
      ...agent,
      kind: "agent",
      avatarKey: agent.key,
    }));
  }

  const visible: SidebarAgentRow[] = [];
  const generated: Aggregate[] = [];
  const system: Aggregate[] = [];
  for (const agent of agents) {
    if (isGeneratedAgentId(agent.key)) {
      generated.push(agent);
    } else if (isInternalAgentId(agent.key)) {
      system.push(agent);
    } else {
      visible.push({ ...agent, kind: "agent", avatarKey: agent.key });
    }
  }

  if (generated.length > 0) {
    visible.push(groupAgents("generated", "sidebar.agent_generated", "generated", generated));
  }
  if (system.length > 0) {
    visible.push(groupAgents("system", "sidebar.agent_system", "system", system));
  }
  return visible;
}

function groupAgents(
  kind: "generated" | "system",
  labelKey: string,
  avatarKey: string,
  rows: Aggregate[],
): SidebarAgentRow {
  return {
    key: `__${kind}_agents__`,
    kind,
    labelKey,
    avatarKey,
    count: rows.reduce((sum, row) => sum + row.count, 0),
    rawKeys: rows.map((row) => row.key).sort(),
  };
}
