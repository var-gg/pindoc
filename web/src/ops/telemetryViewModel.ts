export type TelemetryRecentTool = {
  tool_name: string;
};

export function telemetrySelectedToolFromSearch(search: string): string {
  return new URLSearchParams(search).get("tool")?.trim() ?? "";
}

export function telemetrySearchForSelectedTool(search: string, toolName: string): string {
  const params = new URLSearchParams(search);
  const selected = toolName.trim();

  if (selected) {
    params.set("tool", selected);
  } else {
    params.delete("tool");
  }

  const nextSearch = params.toString();
  return nextSearch ? `?${nextSearch}` : "";
}

export function toggleTelemetryToolSelection(currentTool: string, nextTool: string): string {
  return currentTool === nextTool ? "" : nextTool;
}

export function filterTelemetryRecentByTool<T extends TelemetryRecentTool>(
  recent: T[],
  selectedTool: string,
): T[] {
  return selectedTool ? recent.filter((call) => call.tool_name === selectedTool) : recent;
}
