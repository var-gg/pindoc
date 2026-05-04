import type { ServerConfig } from "./api/client";

export type FirstRunConfig = Pick<ServerConfig, "identity_required" | "onboarding_required">;

export const FIRST_RUN_CONFIG_CHANGED_EVENT = "pindoc:first-run-config-changed";

export function firstRunRedirectTarget(pathname: string, cfg: FirstRunConfig): string | null {
  const path = normalizeAppPathname(pathname);
  if (cfg.identity_required) {
    return path === "/onboarding/identity" ? null : "/onboarding/identity";
  }
  if (cfg.onboarding_required && path !== "/projects/new") {
    return "/projects/new?welcome=1";
  }
  return null;
}

export function notifyFirstRunConfigChanged(config?: Partial<FirstRunConfig>): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent(FIRST_RUN_CONFIG_CHANGED_EVENT, { detail: config ?? {} }));
}

function normalizeAppPathname(pathname: string): string {
  const path = (pathname || "/").split(/[?#]/, 1)[0].replace(/\/+$/, "");
  return path === "" ? "/" : path;
}
