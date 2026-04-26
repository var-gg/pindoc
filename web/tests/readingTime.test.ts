import { countRunes, estimateReadingTime } from "../src/utils/readingTime";

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function testKoreanRuneEstimate(): void {
  const text = "한국어 본문입니다. 글자 수 기반으로 읽기 시간을 계산합니다.";
  const estimate = estimateReadingTime(text, "ko");
  assertEqual(estimate.profile, "ko", "Korean locale maps to ko profile");
  assertEqual(estimate.runeCount, countRunes(text), "Korean rune count is code-point based");
  assertEqual(estimate.estimatedMinutes, 1, "Short Korean body rounds up to one minute");
}

function testEnglishWordEstimate(): void {
  const estimate = estimateReadingTime("One two three four five.", "en");
  assertEqual(estimate.profile, "en", "English locale maps to en profile");
  assertEqual(estimate.wordCount, 5, "English estimate counts words");
  assertEqual(estimate.estimatedMinutes, 1, "Short English body rounds up to one minute");
}

function testMixedEstimate(): void {
  const estimate = estimateReadingTime("한국어 context with English words.", "ko");
  assertEqual(estimate.profile, "mixed", "Mixed Korean and English text maps to mixed profile");
  assertEqual(estimate.wordCount, 4, "Mixed estimate still counts English words");
  assertEqual(estimate.estimatedMinutes, 1, "Short mixed body rounds up to one minute");
}

testKoreanRuneEstimate();
testEnglishWordEstimate();
testMixedEstimate();
