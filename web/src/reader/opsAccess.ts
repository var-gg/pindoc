export function telemetryDebugEnabled(search: string, storedFlag: string | null | undefined): boolean {
  const params = new URLSearchParams(search.startsWith("?") ? search : `?${search}`);
  return params.get("ops") === "1" || params.get("debug") === "ops" || storedFlag === "1";
}

export function canShowTelemetryNav(
  role: "owner" | "editor" | "viewer" | undefined,
  debugEnabled: boolean,
): boolean {
  return role === "owner" || debugEnabled;
}
