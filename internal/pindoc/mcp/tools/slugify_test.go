package tools

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestSlugify covers the Phase 17 follow-up behaviour: Unicode letters
// survive verbatim, non-letter/non-digit runs collapse to a single
// hyphen, and the cap is rune-based (not byte-based) so multi-byte
// characters don't get chopped mid-code-point.
func TestSlugify(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain ascii",
			in:   "Agent-written wiki",
			want: "agent-written-wiki",
		},
		{
			name: "korean preserved",
			// Was "pindoc-url" under the old ASCII-only policy.
			in:   "Pindoc 시스템 아키텍처 — URL 스코프",
			want: "pindoc-시스템-아키텍처-url-스코프",
		},
		{
			name: "mixed korean + emdash + ascii",
			in:   "Pindoc 제품 비전 — Agent-written 위키의 북극성",
			want: "pindoc-제품-비전-agent-written-위키의-북극성",
		},
		{
			name: "only punctuation yields empty",
			in:   "!!!---???",
			want: "",
		},
		{
			name: "leading and trailing hyphens trimmed",
			in:   "  — a — b —  ",
			want: "a-b",
		},
		{
			name: "lowercases ascii but leaves hangul",
			in:   "ABC 한글",
			want: "abc-한글",
		},
		{
			name: "japanese preserved",
			in:   "プロジェクト ドキュメント",
			want: "プロジェクト-ドキュメント",
		},
		{
			name: "cyrillic preserved",
			in:   "Проект Пиндок",
			want: "проект-пиндок",
		},
		{
			name: "digit runs preserved",
			in:   "v2 release 2026",
			want: "v2-release-2026",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := slugify(tc.in)
			if got != tc.want {
				t.Errorf("slugify(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSlugifyWordBoundaryCap makes sure the cap never cuts through the
// middle of a token. Gives a long Hangul-heavy title and checks the result
// is valid UTF-8 and ends at a full token boundary.
func TestSlugifyWordBoundaryCap(t *testing.T) {
	// 70 runes of 한글 separated by spaces — well over the 60-rune cap.
	long := "테스트 아주 긴 한글 제목의 슬러그 생성이 유니코드 안전하게 잘 되는지 " +
		"확인하기 위한 시험용 입력값이며 60자보다 확실히 길게 만들어둠"
	got := slugify(long)
	if !utf8.ValidString(got) {
		t.Fatalf("slugify produced invalid UTF-8: %q", got)
	}
	if utf8.RuneCountInString(got) > 60 {
		t.Errorf("slugify result has %d runes, want ≤ 60 (%q)",
			utf8.RuneCountInString(got), got)
	}
	if strings.HasSuffix(got, "-") {
		t.Errorf("slugify cut through a token boundary: %q", got)
	}
}

func TestSlugifyPreservesSingleLongKoreanToken(t *testing.T) {
	longToken := "초장문한국어토큰초장문한국어토큰초장문한국어토큰초장문한국어토큰초장문한국어토큰"
	got := slugify("prefix " + longToken)
	if !utf8.ValidString(got) {
		t.Fatalf("slugify produced invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, longToken) {
		t.Fatalf("slugify(%q) = %q; want full Korean token preserved", longToken, got)
	}
}

// TestSlugifyIdempotent: feeding a slug back in should give the same
// slug. Guards against policy bugs where re-slugifying a valid slug
// alters it.
func TestSlugifyIdempotent(t *testing.T) {
	slugs := []string{
		"pindoc-제품-비전-agent-written",
		"abc-한글-123",
		"pure-ascii-slug",
	}
	for _, s := range slugs {
		if got := slugify(s); got != s {
			t.Errorf("slugify(%q) = %q; want unchanged", s, got)
		}
	}
}
