package tools

import "testing"

// TestMarkUncheckedAsDone covers the body rewrite half of pindoc.task.claim_done.
// Only "- [ ]" markers move to "[x]"; "[x]" / "[X]" / "[~]" / "[-]" are
// preserved because they represent prior judgment calls (already done /
// partial / deferred) that an automatic mass-toggle must not overwrite.
func TestMarkUncheckedAsDone(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		want        string
		wantChanged int
	}{
		{
			name:        "empty body",
			body:        "",
			want:        "",
			wantChanged: 0,
		},
		{
			name:        "no checkboxes",
			body:        "## Purpose\n\nJust prose.\n",
			want:        "## Purpose\n\nJust prose.\n",
			wantChanged: 0,
		},
		{
			name:        "single unchecked",
			body:        "- [ ] item",
			want:        "- [x] item",
			wantChanged: 1,
		},
		{
			name:        "multiple unchecked",
			body:        "- [ ] one\n- [ ] two\n- [ ] three",
			want:        "- [x] one\n- [x] two\n- [x] three",
			wantChanged: 3,
		},
		{
			name:        "mixed states preserved",
			body:        "- [ ] todo\n- [x] done\n- [~] partial\n- [-] deferred\n- [ ] more",
			want:        "- [x] todo\n- [x] done\n- [~] partial\n- [-] deferred\n- [x] more",
			wantChanged: 2,
		},
		{
			name:        "uppercase X is preserved as-is",
			body:        "- [X] capital done\n- [ ] todo",
			want:        "- [X] capital done\n- [x] todo",
			wantChanged: 1,
		},
		{
			name:        "asterisk and plus bullets accepted",
			body:        "* [ ] star\n+ [ ] plus\n- [ ] dash",
			want:        "* [x] star\n+ [x] plus\n- [x] dash",
			wantChanged: 3,
		},
		{
			name:        "indented checkbox",
			body:        "  - [ ] indented\n    - [ ] deeper",
			want:        "  - [x] indented\n    - [x] deeper",
			wantChanged: 2,
		},
		{
			name:        "all already resolved no change",
			body:        "- [x] one\n- [~] two\n- [-] three",
			want:        "- [x] one\n- [~] two\n- [-] three",
			wantChanged: 0,
		},
		{
			name:        "non-checkbox brackets ignored",
			body:        "- not a checkbox\n[ ] no bullet either\n- [ ] real one",
			want:        "- not a checkbox\n[ ] no bullet either\n- [x] real one",
			wantChanged: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, changed := markUncheckedAsDone(c.body)
			if got != c.want {
				t.Fatalf("markUncheckedAsDone body:\n--- got ---\n%s\n--- want ---\n%s", got, c.want)
			}
			if changed != c.wantChanged {
				t.Fatalf("markUncheckedAsDone changed: got %d, want %d", changed, c.wantChanged)
			}
		})
	}
}
