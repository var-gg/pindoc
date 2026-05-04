import { firstRunRedirectTarget } from "../src/firstRunConfig";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testIdentityRequiredInterceptsEveryTopLevelSurface(): void {
  for (const path of ["/p/pindoc/today", "/projects/new", "/admin/providers"]) {
    assertEqual(
      firstRunRedirectTarget(path, { identity_required: true, onboarding_required: true }),
      "/onboarding/identity",
      `${path} should route to identity before any project chrome loads`,
    );
  }
  assertEqual(
    firstRunRedirectTarget("/onboarding/identity", { identity_required: true, onboarding_required: true }),
    null,
    "identity route should be reachable while identity setup is required",
  );
}

function testOnboardingRequiredInterceptsFreshDeepLinks(): void {
  assertEqual(
    firstRunRedirectTarget("/p/pindoc/today", { identity_required: false, onboarding_required: true }),
    "/projects/new?welcome=1",
    "fresh deep link should route to the project wizard",
  );
  assertEqual(
    firstRunRedirectTarget("/projects/new?welcome=1", { identity_required: false, onboarding_required: true }),
    null,
    "project wizard should be reachable while onboarding is required",
  );
}

function testCompletedIdentityDoesNotRedirect(): void {
  assertEqual(
    firstRunRedirectTarget("/p/pindoc/today", { identity_required: false, onboarding_required: false }),
    null,
    "completed identity setup should leave project links alone",
  );
  assertEqual(
    firstRunRedirectTarget("/projects/new?welcome=1", { identity_required: false, onboarding_required: false }),
    null,
    "completed identity setup should leave project create links alone",
  );
}

testIdentityRequiredInterceptsEveryTopLevelSurface();
testOnboardingRequiredInterceptsFreshDeepLinks();
testCompletedIdentityDoesNotRedirect();
