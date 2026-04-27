package tools

import "testing"

func TestReceiptExemptionFromStats(t *testing.T) {
	cases := []struct {
		name          string
		total         int
		otherAuthors  int
		limit         int
		wantOK        bool
		wantRemaining int
	}{
		{"first empty-area write", 0, 0, 5, true, 4},
		{"last allowed write", 4, 0, 5, true, 0},
		{"N plus one requires receipt", 5, 0, 5, false, 0},
		{"other author disables bootstrap exemption", 1, 1, 5, false, 0},
		{"zero limit disables exemption", 0, 0, 0, false, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := receiptExemptionFromStats(c.total, c.otherAuthors, c.limit)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (signal=%+v)", ok, c.wantOK, got)
			}
			if !ok {
				return
			}
			if got.Reason != "empty_area_first_proposes" {
				t.Fatalf("reason = %q", got.Reason)
			}
			if got.NRemaining != c.wantRemaining {
				t.Fatalf("n_remaining = %d, want %d", got.NRemaining, c.wantRemaining)
			}
			if got.Limit != c.limit {
				t.Fatalf("limit = %d, want %d", got.Limit, c.limit)
			}
		})
	}
}
