package projects

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestWebProjectSlugPolicyMirror(t *testing.T) {
	path := filepath.Join("..", "..", "..", "web", "src", "reader", "projectSlugPolicy.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read web slug policy mirror: %v", err)
	}
	var mirror struct {
		Pattern       string   `json:"pattern"`
		HTMLPattern   string   `json:"htmlPattern"`
		ReservedSlugs []string `json:"reservedSlugs"`
	}
	if err := json.Unmarshal(data, &mirror); err != nil {
		t.Fatalf("decode web slug policy mirror: %v", err)
	}
	if mirror.Pattern != projectSlugRe.String() {
		t.Fatalf("web slug pattern = %q, want %q", mirror.Pattern, projectSlugRe.String())
	}
	if mirror.HTMLPattern != "[a-z][a-z0-9-]{1,39}" {
		t.Fatalf("web HTML pattern = %q, want browser-safe unanchored pattern", mirror.HTMLPattern)
	}

	wantReserved := make([]string, 0, len(reservedSlugs))
	for slug := range reservedSlugs {
		wantReserved = append(wantReserved, slug)
	}
	sort.Strings(wantReserved)
	gotReserved := append([]string(nil), mirror.ReservedSlugs...)
	sort.Strings(gotReserved)
	if !reflect.DeepEqual(gotReserved, wantReserved) {
		t.Fatalf("web reserved slugs drifted\n got: %#v\nwant: %#v", gotReserved, wantReserved)
	}
}
