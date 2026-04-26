export type ReadingProfile = "ko" | "en" | "mixed";

export type ReadingEstimate = {
  profile: ReadingProfile;
  runeCount: number;
  wordCount: number;
  estimatedMinutes: number;
};

const KO_CHARS_PER_MINUTE = 420;
const EN_WORDS_PER_MINUTE = 265;

const cjkRegex = /[\u1100-\u11ff\u3130-\u318f\uac00-\ud7af\u3040-\u30ff\u3400-\u9fff]/u;
const cjkGlobalRegex = /[\u1100-\u11ff\u3130-\u318f\uac00-\ud7af\u3040-\u30ff\u3400-\u9fff]/gu;
const wordRegex = /[A-Za-z0-9]+(?:['-][A-Za-z0-9]+)*/g;

export function countRunes(text: string): number {
  return Array.from(stripMarkdownNoise(text).replace(/\s+/g, "")).length;
}

export function countEnglishWords(text: string): number {
  return stripMarkdownNoise(text).match(wordRegex)?.length ?? 0;
}

export function estimateReadingTime(text: string, locale?: string): ReadingEstimate {
  const normalized = stripMarkdownNoise(text);
  const runeCount = countRunes(normalized);
  const wordCount = countEnglishWords(normalized);
  const cjkCount = normalized.match(cjkGlobalRegex)?.length ?? 0;
  const localeLower = (locale ?? "").toLowerCase();
  const hasCJK = cjkRegex.test(normalized);
  const hasEnglishWords = wordCount > 0;
  const profile: ReadingProfile =
    hasCJK && hasEnglishWords
      ? "mixed"
      : localeLower.startsWith("ko") || hasCJK
        ? "ko"
        : "en";

  let minutes = 0;
  if (profile === "en") {
    minutes = wordCount / EN_WORDS_PER_MINUTE;
  } else if (profile === "ko") {
    minutes = runeCount / KO_CHARS_PER_MINUTE;
  } else {
    const nonCJKText = normalized.replace(cjkGlobalRegex, " ");
    const mixedWordCount = countEnglishWords(nonCJKText);
    minutes = cjkCount / KO_CHARS_PER_MINUTE + mixedWordCount / EN_WORDS_PER_MINUTE;
  }

  return {
    profile,
    runeCount,
    wordCount,
    estimatedMinutes: runeCount === 0 ? 0 : Math.max(1, Math.ceil(minutes)),
  };
}

function stripMarkdownNoise(text: string): string {
  return text
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/`([^`]+)`/g, "$1")
    .replace(/!\[[^\]]*]\([^)]*\)/g, " ")
    .replace(/\[([^\]]+)]\([^)]*\)/g, "$1")
    .replace(/[#>*_~|\[\]()-]/g, " ");
}
