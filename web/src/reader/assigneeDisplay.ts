export function taskAssigneeLabel(
  value: string | undefined,
  t: (key: string, ...args: Array<string | number>) => string,
): string {
  const trimmed = value?.trim() ?? "";
  if (!trimmed) return t("tasks.assignee_unassigned");
  if (trimmed.startsWith("user:")) {
    return trimmed.slice("user:".length).trim() || t("tasks.assignee_unassigned");
  }
  return trimmed;
}

export function taskAssigneeActorKey(value: string | undefined): string {
  return (value ?? "")
    .trim()
    .toLowerCase()
    .replace(/^@/, "")
    .replace(/^agent:/, "")
    .replace(/^user:/, "");
}
