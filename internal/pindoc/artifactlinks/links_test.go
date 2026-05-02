package artifactlinks

import (
	"errors"
	"strings"
	"testing"
)

func TestRewritePindocLinksSkipsCode(t *testing.T) {
	body := strings.Join([]string{
		"See pindoc://target-one.",
		"Inline `pindoc://literal` stays.",
		"```",
		"pindoc://literal-fence",
		"```",
	}, "\n")
	got, refs, err := RewritePindocLinks(body, func(ref string) (string, error) {
		return "/p/pindoc/wiki/" + ref, nil
	})
	if err != nil {
		t.Fatalf("RewritePindocLinks err = %v", err)
	}
	if !strings.Contains(got, "See /p/pindoc/wiki/target-one.") {
		t.Fatalf("rewritten body missing normalized link:\n%s", got)
	}
	for _, literal := range []string{"`pindoc://literal`", "pindoc://literal-fence"} {
		if !strings.Contains(got, literal) {
			t.Fatalf("rewritten body should preserve %q:\n%s", literal, got)
		}
	}
	if len(refs) != 1 || refs[0] != "target-one" {
		t.Fatalf("refs = %#v, want target-one", refs)
	}
}

func TestRewritePindocLinksReportsInvalidReference(t *testing.T) {
	wantErr := errors.New("missing")
	_, _, err := RewritePindocLinks("See pindoc://missing", func(ref string) (string, error) {
		return "", wantErr
	})
	var linkErr *LinkError
	if !errors.As(err, &linkErr) {
		t.Fatalf("err = %T %[1]v, want LinkError", err)
	}
	if linkErr.Ref != "missing" || !errors.Is(linkErr, wantErr) {
		t.Fatalf("linkErr = ref %q err %v", linkErr.Ref, linkErr.Err)
	}
}
