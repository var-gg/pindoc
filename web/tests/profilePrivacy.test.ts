import { maskEmail } from "../src/reader/profilePrivacy";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testEmailMaskKeepsDomainButHidesLocalPart(): void {
  assertEqual(maskEmail("alice@example.com"), "al***@example.com", "normal email mask");
  assertEqual(maskEmail("a@example.com"), "a***@example.com", "short local mask");
  assertEqual(maskEmail("local-only"), "lo***", "non-email fallback mask");
}

testEmailMaskKeepsDomainButHidesLocalPart();
