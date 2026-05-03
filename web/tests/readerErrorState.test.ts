import en = require("../src/i18n/en.json");
import ko = require("../src/i18n/ko.json");
import {
  classifyReaderError,
  readerErrorTitleKey,
  shouldShowReaderErrorDevHint,
} from "../src/reader/readerErrorState";

const enBundle = en as Record<string, string>;
const koBundle = ko as Record<string, string>;

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testReaderErrorClassifierSeparates404(): void {
  const notFound = classifyReaderError('Error: 404 Not Found: {"error":"artifact not found"}');
  const generic = classifyReaderError("TypeError: Failed to fetch");

  assertEqual(notFound, "not_found", "404 artifact fetch errors should be not_found");
  assertEqual(generic, "generic", "network errors should be generic");
  assertEqual(
    readerErrorTitleKey(notFound),
    "wiki.error_not_found_title",
    "404 errors should use the not-found title key",
  );
  assertEqual(
    readerErrorTitleKey(generic),
    "wiki.error_generic_title",
    "generic errors should use the generic title key",
  );
}

function testDevHintOnlyForGenericDevErrors(): void {
  assert(!shouldShowReaderErrorDevHint("not_found", true), "404 errors must not show a dev hint");
  assert(shouldShowReaderErrorDevHint("generic", true), "generic dev errors should show a dev hint");
  assert(!shouldShowReaderErrorDevHint("generic", false), "prod builds should hide the dev hint");
}

function testReaderErrorI18nKeysLandWithoutLegacyHints(): void {
  const required = [
    "wiki.error_not_found_title",
    "wiki.error_generic_title",
    "wiki.error_dev_hint_prefix",
    "wiki.error_dev_hint_cmd",
    "wiki.error_dev_hint_suffix",
    "wiki.error_back_to_project",
  ];
  for (const key of required) {
    assert(Boolean(enBundle[key]), `EN bundle should include ${key}`);
    assert(Boolean(koBundle[key]), `KO bundle should include ${key}`);
  }

  for (const bundle of [enBundle, koBundle]) {
    for (const suffix of ["title", "hint_prefix", "hint_cmd", "hint_suffix"]) {
      const legacyKey = "wiki." + "error_" + suffix;
      assert(!(legacyKey in bundle), `legacy key should be removed: ${legacyKey}`);
    }
    const joined = required.map((key) => bundle[key]).join("\n");
    assert(!joined.includes("pindoc-api"), "dev hint should not mention pindoc-api");
    assert(!joined.includes("5831"), "dev hint should not mention port 5831");
  }
}

testReaderErrorClassifierSeparates404();
testDevHintOnlyForGenericDevErrors();
testReaderErrorI18nKeysLandWithoutLegacyHints();
