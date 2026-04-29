import { maskEmail } from "../src/reader/profilePrivacy";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testEmailMaskKeepsDomainButHidesLocalPart(): void {
  assertEqual(maskEmail("rhkdwls750@naver.com"), "rh***@naver.com", "normal email mask");
  assertEqual(maskEmail("a@example.com"), "a***@example.com", "short local mask");
  assertEqual(maskEmail("local-only"), "lo***", "non-email fallback mask");
}

testEmailMaskKeepsDomainButHidesLocalPart();
