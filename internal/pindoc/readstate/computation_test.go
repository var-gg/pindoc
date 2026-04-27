package readstate

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestExpectedSeconds_Korean(t *testing.T) {
	// 600 ko chars at 600 chars/min → 60s
	body := strings.Repeat("가", 600)
	got := ExpectedSeconds(body, "ko")
	if math.Abs(got-60.0) > 0.01 {
		t.Errorf("ExpectedSeconds(600 ko) = %f, want 60", got)
	}
}

func TestExpectedSeconds_English(t *testing.T) {
	// 1250 en chars at 1250 chars/min → 60s
	body := strings.Repeat("a", 1250)
	got := ExpectedSeconds(body, "en")
	if math.Abs(got-60.0) > 0.01 {
		t.Errorf("ExpectedSeconds(1250 en) = %f, want 60", got)
	}
}

func TestExpectedSeconds_KoreanLocaleVariants(t *testing.T) {
	body := strings.Repeat("가", 600)
	for _, locale := range []string{"ko", "ko-KR", "ko_KR", "KO", " ko "} {
		if got := ExpectedSeconds(body, locale); math.Abs(got-60.0) > 0.01 {
			t.Errorf("locale %q gave %f, want 60 (Korean speed)", locale, got)
		}
	}
}

func TestExpectedSeconds_Floor(t *testing.T) {
	short := ExpectedSeconds("안녕", "ko")
	if short != minExpectedSec {
		t.Errorf("short body should hit floor %f, got %f", minExpectedSec, short)
	}
	empty := ExpectedSeconds("", "ko")
	if empty != minExpectedSec {
		t.Errorf("empty body should hit floor %f, got %f", minExpectedSec, empty)
	}
}

func TestCompletion_FullRead(t *testing.T) {
	if got := Completion(60.0, 1.0, 60.0); got != 1.0 {
		t.Errorf("Completion(full) = %f, want 1.0", got)
	}
}

func TestCompletion_DwellAndScrollMultiply(t *testing.T) {
	if got := Completion(30.0, 1.0, 60.0); math.Abs(got-0.5) > 0.001 {
		t.Errorf("half dwell × full scroll = %f, want 0.5", got)
	}
	if got := Completion(60.0, 0.5, 60.0); math.Abs(got-0.5) > 0.001 {
		t.Errorf("full dwell × half scroll = %f, want 0.5", got)
	}
}

func TestCompletion_DwellSaturates(t *testing.T) {
	// active > expected does not push completion above scroll_max
	if got := Completion(120.0, 0.7, 60.0); math.Abs(got-0.7) > 0.001 {
		t.Errorf("over-dwell with scroll 0.7 = %f, want 0.7", got)
	}
}

func TestCompletion_NegativeSafety(t *testing.T) {
	if got := Completion(-1.0, -0.5, 60.0); got != 0 {
		t.Errorf("negative inputs should clamp to 0, got %f", got)
	}
	if got := Completion(60.0, 1.5, 60.0); got != 1.0 {
		t.Errorf("scroll > 1 should clamp, got %f", got)
	}
}

func TestCompletion_ZeroExpected(t *testing.T) {
	if got := Completion(60.0, 1.0, 0); got != 0 {
		t.Errorf("zero expected → 0 completion, got %f", got)
	}
}

func TestClassify_Unseen(t *testing.T) {
	if got := Classify(0, 0, 60); got != StateUnseen {
		t.Errorf("0/0 → unseen, got %v", got)
	}
}

func TestClassify_GlancedBelowReadThreshold(t *testing.T) {
	// active=10, scroll=0.3, expected=60 → 10/60 × 0.3 ≈ 0.05 < 0.5
	if got := Classify(10, 0.3, 60); got != StateGlanced {
		t.Errorf("low signals → glanced, got %v", got)
	}
}

func TestClassify_ReadAtThreshold(t *testing.T) {
	// active=30, scroll=1.0, expected=60 → exactly 0.5
	if got := Classify(30, 1.0, 60); got != StateRead {
		t.Errorf("0.5 boundary → read, got %v", got)
	}
}

func TestClassify_DeeplyReadAtThreshold(t *testing.T) {
	// active=60 (saturated), scroll=0.8, expected=60 → exactly 0.8
	if got := Classify(60, 0.8, 60); got != StateDeeplyRead {
		t.Errorf("0.8 boundary → deeply_read, got %v", got)
	}
}

func TestClassify_ScrollOnlyStaysGlanced(t *testing.T) {
	// scroll fast to bottom, no dwell — should stay glanced
	if got := Classify(0.5, 1.0, 60); got != StateGlanced {
		t.Errorf("scroll-only → glanced, got %v", got)
	}
}

func TestClassify_DwellOnlyStaysGlanced(t *testing.T) {
	// active > 0 takes us out of the unseen branch; completion = 1 × 0 = 0,
	// which lands below the read threshold → glanced.
	if got := Classify(60, 0.0, 60); got != StateGlanced {
		t.Errorf("dwell + zero scroll → glanced, got %v", got)
	}
}

func TestClassifyAggregate_NilGivesUnseen(t *testing.T) {
	s := ClassifyAggregate(nil, "body", "ko")
	if s.ReadState != StateUnseen {
		t.Errorf("nil agg → unseen, got %v", s.ReadState)
	}
	if s.CompletionPct != 0 {
		t.Errorf("nil agg → 0 completion, got %f", s.CompletionPct)
	}
	if s.LastSeenAt != nil {
		t.Errorf("nil agg → nil LastSeenAt, got %v", s.LastSeenAt)
	}
}

func TestClassifyAggregate_ZeroEventCountGivesUnseen(t *testing.T) {
	agg := &Aggregate{ArtifactID: "art-1", UserKey: "local", EventCount: 0}
	s := ClassifyAggregate(agg, "body", "ko")
	if s.ReadState != StateUnseen {
		t.Errorf("0 events → unseen, got %v", s.ReadState)
	}
}

func TestClassifyAggregate_DeepReadKorean(t *testing.T) {
	body := strings.Repeat("가", 600) // 60s expected
	last := time.Date(2026, 4, 27, 13, 0, 0, 0, time.UTC)
	agg := &Aggregate{
		ArtifactID:         "art-1",
		UserKey:            "local",
		TotalActiveSeconds: 60,
		MaxScrollPct:       0.9,
		EventCount:         3,
		LastSeenAt:         last,
	}
	s := ClassifyAggregate(agg, body, "ko")
	if s.ReadState != StateDeeplyRead {
		t.Errorf("60s × 0.9 over 60s expected → deeply_read, got %v", s.ReadState)
	}
	if s.CompletionPct < 0.89 || s.CompletionPct > 0.91 {
		t.Errorf("completion ~0.9, got %f", s.CompletionPct)
	}
	if s.LastSeenAt == nil || !s.LastSeenAt.Equal(last) {
		t.Errorf("LastSeenAt not propagated correctly: %v", s.LastSeenAt)
	}
	if s.ArtifactID != "art-1" || s.UserKey != "local" {
		t.Errorf("identity not propagated: %+v", s)
	}
}

func TestIsKoreanLocale(t *testing.T) {
	cases := map[string]bool{
		"ko":    true,
		"ko-KR": true,
		"ko_KR": true,
		"KO":    true,
		" ko ":  true,
		"en":    false,
		"en-US": false,
		"ja":    false,
		"":      false,
	}
	for input, want := range cases {
		if got := isKoreanLocale(input); got != want {
			t.Errorf("isKoreanLocale(%q) = %v, want %v", input, got, want)
		}
	}
}
