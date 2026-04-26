package httpapi

import (
	"strings"
	"testing"
	"time"
)

func TestValidateReadEvent(t *testing.T) {
	start := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	valid := readEventRequest{
		ArtifactID:    "5a82d553-48f6-4473-9eb3-930e1fa3e029",
		StartedAt:     start,
		EndedAt:       start.Add(2 * time.Minute),
		ActiveSeconds: 90,
		ScrollMaxPct:  0.82,
		IdleSeconds:   30,
		Locale:        "ko",
	}
	if err := validateReadEvent(valid); err != nil {
		t.Fatalf("valid read event rejected: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*readEventRequest)
		want string
	}{
		{
			name: "missing artifact ref",
			mut: func(in *readEventRequest) {
				in.ArtifactID = ""
				in.ArtifactSlug = ""
			},
			want: "artifact_id or artifact_slug",
		},
		{
			name: "ended before started",
			mut: func(in *readEventRequest) {
				in.EndedAt = in.StartedAt.Add(-time.Second)
			},
			want: "ended_at must be after started_at",
		},
		{
			name: "active exceeds wall duration",
			mut: func(in *readEventRequest) {
				in.ActiveSeconds = 121
			},
			want: "active_seconds must not exceed session duration",
		},
		{
			name: "scroll below zero",
			mut: func(in *readEventRequest) {
				in.ScrollMaxPct = -0.01
			},
			want: "scroll_max_pct must be between 0 and 1",
		},
		{
			name: "scroll above one",
			mut: func(in *readEventRequest) {
				in.ScrollMaxPct = 1.01
			},
			want: "scroll_max_pct must be between 0 and 1",
		},
		{
			name: "negative idle",
			mut: func(in *readEventRequest) {
				in.IdleSeconds = -1
			},
			want: "idle_seconds must be non-negative",
		},
		{
			name: "locale too long",
			mut: func(in *readEventRequest) {
				in.Locale = "ko-Kore-KR-too-long"
			},
			want: "locale is too long",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input := valid
			c.mut(&input)
			err := validateReadEvent(input)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), c.want)
			}
		})
	}
}
