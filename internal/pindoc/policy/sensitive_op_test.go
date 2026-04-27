package policy

import "testing"

func TestIsSensitive(t *testing.T) {
	cases := []struct {
		name string
		op   SensitiveOp
		ctx  SensitiveContext
		want bool
	}{
		{name: "delete", op: OpDelete, want: true},
		{name: "archive", op: OpArchive, want: true},
		{name: "supersede", op: OpSupersede, want: true},
		{name: "new area", op: OpNewArea, want: true},
		{name: "force flag", op: OpNone, ctx: SensitiveContext{Force: true}, want: true},
		{name: "force op", op: OpForce, want: true},
		{name: "bulk supersede below threshold", op: OpBulkSupersede, ctx: SensitiveContext{SupersedeCount: 2}, want: false},
		{name: "bulk supersede at threshold", op: OpBulkSupersede, ctx: SensitiveContext{SupersedeCount: 3}, want: true},
		{name: "settled promotion", op: OpSettledPromotion, ctx: SensitiveContext{FromCompleteness: "partial", ToCompleteness: "settled"}, want: true},
		{name: "settled remains settled", op: OpSettledPromotion, ctx: SensitiveContext{FromCompleteness: "settled", ToCompleteness: "settled"}, want: false},
		{name: "partial write is normal", op: OpCompletenessWrite, ctx: SensitiveContext{FromCompleteness: "draft", ToCompleteness: "partial"}, want: false},
		{name: "normal op", op: OpNone, want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsSensitive(c.op, c.ctx); got != c.want {
				t.Fatalf("IsSensitive(%q, %+v) = %v, want %v", c.op, c.ctx, got, c.want)
			}
		})
	}
}

func TestReviewStateFor(t *testing.T) {
	ctx := SensitiveContext{FromCompleteness: "partial", ToCompleteness: "settled"}
	if got := ReviewStateFor(SensitiveOpsConfirm, OpSettledPromotion, ctx); got != ReviewStatePending {
		t.Fatalf("confirm sensitive review state = %q, want %q", got, ReviewStatePending)
	}
	if got := ReviewStateFor(SensitiveOpsAuto, OpSettledPromotion, ctx); got != ReviewStateAutoPublished {
		t.Fatalf("auto sensitive review state = %q, want %q", got, ReviewStateAutoPublished)
	}
	if got := ReviewStateFor(SensitiveOpsConfirm, OpNone, SensitiveContext{}); got != ReviewStateAutoPublished {
		t.Fatalf("confirm normal review state = %q, want %q", got, ReviewStateAutoPublished)
	}
}
