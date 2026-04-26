package policy

import "strings"

const (
	SensitiveOpsAuto    = "auto"
	SensitiveOpsConfirm = "confirm"

	ReviewStateAutoPublished = "auto_published"
	ReviewStatePending       = "pending_review"

	BulkSupersedeThreshold = 3
)

type SensitiveOp string

const (
	OpNone              SensitiveOp = ""
	OpDelete            SensitiveOp = "delete"
	OpArchive           SensitiveOp = "archive"
	OpSupersede         SensitiveOp = "supersede"
	OpSettledPromotion  SensitiveOp = "settled_promotion"
	OpNewArea           SensitiveOp = "new_area"
	OpForce             SensitiveOp = "force"
	OpBulkSupersede     SensitiveOp = "bulk_supersede"
	OpCompletenessWrite SensitiveOp = "completeness_write"
)

type SensitiveContext struct {
	FromCompleteness string
	ToCompleteness   string
	SupersedeCount   int
	BulkThreshold    int
	Force            bool
}

func NormalizeSensitiveOpsMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case SensitiveOpsConfirm:
		return SensitiveOpsConfirm
	default:
		return SensitiveOpsAuto
	}
}

func IsSensitive(op SensitiveOp, ctx SensitiveContext) bool {
	if ctx.Force || op == OpForce {
		return true
	}
	switch op {
	case OpDelete, OpArchive, OpSupersede, OpNewArea:
		return true
	case OpSettledPromotion, OpCompletenessWrite:
		return promotesToSettled(ctx.FromCompleteness, ctx.ToCompleteness)
	case OpBulkSupersede:
		threshold := ctx.BulkThreshold
		if threshold <= 0 {
			threshold = BulkSupersedeThreshold
		}
		return ctx.SupersedeCount >= threshold
	default:
		return false
	}
}

func ReviewStateFor(mode string, op SensitiveOp, ctx SensitiveContext) string {
	if NormalizeSensitiveOpsMode(mode) == SensitiveOpsConfirm && IsSensitive(op, ctx) {
		return ReviewStatePending
	}
	return ReviewStateAutoPublished
}

func promotesToSettled(from, to string) bool {
	to = strings.ToLower(strings.TrimSpace(to))
	if to != "settled" {
		return false
	}
	from = strings.ToLower(strings.TrimSpace(from))
	return from != "settled"
}
