package tools

import "testing"

func TestHumanURLCanonicalProjectPath(t *testing.T) {
	got := HumanURL("pindoc", "ko", "canonical-only-on-demand-translation")
	want := "/p/pindoc/wiki/canonical-only-on-demand-translation"
	if got != want {
		t.Fatalf("HumanURL = %q; want %q", got, want)
	}
}
