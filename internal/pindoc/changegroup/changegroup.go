package changegroup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

const defaultWindow = 15 * time.Minute

type Options struct {
	Limit           int
	AreaSlug        string
	Kind            string
	SinceRevisionID int
	SinceTime       *time.Time
	Window          time.Duration
}

type RevisionRow struct {
	ProjectSlug      string
	RevisionID       string
	ArtifactID       string
	ArtifactSlug     string
	ArtifactTitle    string
	ArtifactType     string
	AreaSlug         string
	RevisionNumber   int
	AuthorID         string
	CommitMsg        string
	RevisionShape    string
	SourceSessionRef json.RawMessage
	ArtifactMeta     json.RawMessage
	TaskMeta         json.RawMessage
	CreatedAt        time.Time
}

type GroupingKey struct {
	Kind       string `json:"kind"`
	Value      string `json:"value"`
	Confidence string `json:"confidence"`
}

type Importance struct {
	Score   int      `json:"score"`
	Level   string   `json:"level"`
	Reasons []string `json:"reasons,omitempty"`
}

type Group struct {
	GroupID           string      `json:"group_id"`
	GroupKind         string      `json:"group_kind"`
	GroupingKey       GroupingKey `json:"grouping_key"`
	CommitSummary     string      `json:"commit_summary"`
	RevisionCount     int         `json:"revision_count"`
	ArtifactCount     int         `json:"artifact_count"`
	Areas             []string    `json:"areas"`
	Authors           []string    `json:"authors"`
	TimeStart         time.Time   `json:"time_start"`
	TimeEnd           time.Time   `json:"time_end"`
	Importance        Importance  `json:"importance"`
	VerificationState string      `json:"verification_state"`
}

type CompactGroup struct {
	GroupID       string     `json:"group_id"`
	Kind          string     `json:"kind"`
	CommitSummary string     `json:"commit_summary"`
	Time          time.Time  `json:"time"`
	ArtifactCount int        `json:"artifact_count"`
	Areas         []string   `json:"areas"`
	Importance    Importance `json:"importance"`
}

type Brief struct {
	Headline  string    `json:"headline"`
	Bullets   []string  `json:"bullets"`
	Source    string    `json:"source"`
	AIHint    string    `json:"ai_hint,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Baseline struct {
	RevisionWatermark int        `json:"revision_watermark"`
	LastSeenAt        *time.Time `json:"last_seen_at,omitempty"`
	DefaultedToDays   int        `json:"defaulted_to_days,omitempty"`
}

type QueryResult struct {
	Groups        []Group  `json:"groups"`
	MaxRevisionID int      `json:"max_revision_id"`
	Baseline      Baseline `json:"baseline"`
}

func Query(ctx context.Context, pool *db.Pool, projectSlug string, opts Options) ([]Group, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.Limit > 200 {
		opts.Limit = 200
	}
	window := opts.Window
	if window <= 0 {
		window = defaultWindow
	}
	var since any
	if opts.SinceTime != nil {
		since = *opts.SinceTime
	}
	rows, err := pool.Query(ctx, `
		SELECT
			p.slug,
			r.id::text,
			a.id::text,
			a.slug,
			a.title,
			a.type,
			ar.slug,
			r.revision_number,
			r.author_id,
			COALESCE(r.commit_msg, ''),
			COALESCE(r.revision_shape, ''),
			COALESCE(r.source_session_ref, '{}'::jsonb),
			COALESCE(a.artifact_meta, '{}'::jsonb),
			COALESCE(a.task_meta, '{}'::jsonb),
			r.created_at
		FROM artifact_revisions r
		JOIN artifacts a ON a.id = r.artifact_id
		JOIN projects p ON p.id = a.project_id
		JOIN areas ar ON ar.id = a.area_id
		WHERE p.slug = $1
		  AND a.status <> 'archived'
		  AND ($2::text = '' OR ar.slug = $2)
		  AND ($3::int = 0 OR r.revision_number > $3)
		  AND ($4::timestamptz IS NULL OR r.created_at >= $4)
		ORDER BY r.created_at DESC
		LIMIT $5
	`, projectSlug, opts.AreaSlug, opts.SinceRevisionID, since, opts.Limit*4)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	revisions := []RevisionRow{}
	for rows.Next() {
		var rr RevisionRow
		if err := rows.Scan(
			&rr.ProjectSlug,
			&rr.RevisionID,
			&rr.ArtifactID,
			&rr.ArtifactSlug,
			&rr.ArtifactTitle,
			&rr.ArtifactType,
			&rr.AreaSlug,
			&rr.RevisionNumber,
			&rr.AuthorID,
			&rr.CommitMsg,
			&rr.RevisionShape,
			&rr.SourceSessionRef,
			&rr.ArtifactMeta,
			&rr.TaskMeta,
			&rr.CreatedAt,
		); err != nil {
			return nil, err
		}
		revisions = append(revisions, rr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	groups := GroupRows(revisions, Options{Limit: opts.Limit, Kind: opts.Kind, Window: window})
	return groups, nil
}

func ProjectRevisionWatermark(ctx context.Context, pool *db.Pool, projectSlug string) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM artifact_revisions r
		JOIN artifacts a ON a.id = r.artifact_id
		JOIN projects p ON p.id = a.project_id
		WHERE p.slug = $1 AND a.status <> 'archived'
	`, projectSlug).Scan(&count)
	return count, err
}

func GroupRows(rows []RevisionRow, opts Options) []Group {
	window := opts.Window
	if window <= 0 {
		window = defaultWindow
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].CreatedAt.Before(rows[j].CreatedAt)
	})

	accs := map[string]*groupAcc{}
	for _, row := range rows {
		key, windowStart := groupingKey(row, window)
		groupID := syntheticGroupID(row.ProjectSlug, key.Kind, key.Value, windowStart)
		acc, ok := accs[groupID]
		if !ok {
			acc = newGroupAcc(groupID, key, row.CreatedAt)
			accs[groupID] = acc
		}
		acc.add(row)
	}

	out := make([]Group, 0, len(accs))
	for _, acc := range accs {
		g := acc.finish()
		if opts.Kind != "" && g.GroupKind != opts.Kind {
			continue
		}
		out = append(out, g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Importance.Score != out[j].Importance.Score {
			return out[i].Importance.Score > out[j].Importance.Score
		}
		if !out[i].TimeEnd.Equal(out[j].TimeEnd) {
			return out[i].TimeEnd.After(out[j].TimeEnd)
		}
		return out[i].GroupID < out[j].GroupID
	})
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out
}

func Compact(groups []Group, limit int) []CompactGroup {
	if limit <= 0 || limit > len(groups) {
		limit = len(groups)
	}
	out := make([]CompactGroup, 0, limit)
	for _, g := range groups[:limit] {
		out = append(out, CompactGroup{
			GroupID:       g.GroupID,
			Kind:          g.GroupKind,
			CommitSummary: g.CommitSummary,
			Time:          g.TimeEnd,
			ArtifactCount: g.ArtifactCount,
			Areas:         append([]string{}, g.Areas...),
			Importance:    g.Importance,
		})
	}
	return out
}

func BuildTemplateBrief(groups []Group, locale string) Brief {
	now := time.Now().UTC()
	ko := strings.HasPrefix(strings.ToLower(locale), "ko")
	if len(groups) == 0 {
		if ko {
			return Brief{
				Headline:  "오늘 새 변경은 없습니다.",
				Bullets:   []string{"검토할 Change Group 없음", "읽지 않은 검증 실패 신호 없음", "필요하면 Wiki나 Task 화면에서 전체 이력을 확인"},
				Source:    "rule_based",
				AIHint:    "rule-based",
				CreatedAt: now,
			}
		}
		return Brief{
			Headline:  "No new changes today.",
			Bullets:   []string{"No Change Groups to review", "No unread verification failures", "Open Wiki or Tasks for the full history"},
			Source:    "rule_based",
			AIHint:    "rule-based",
			CreatedAt: now,
		}
	}

	top := groups[0]
	totalRevs := 0
	totalArtifacts := 0
	verificationRisk := false
	for _, g := range groups {
		totalRevs += g.RevisionCount
		totalArtifacts += g.ArtifactCount
		if g.VerificationState == "unverified" || g.VerificationState == "partially_verified" {
			verificationRisk = true
		}
	}
	if ko {
		bullets := []string{
			fmt.Sprintf("가장 중요한 묶음: %s", top.CommitSummary),
			fmt.Sprintf("%d개 그룹, %d개 리비전, %d개 artifact", len(groups), totalRevs, totalArtifacts),
		}
		if verificationRisk {
			bullets = append(bullets, "검증 보강이 필요한 변경 포함")
		} else {
			bullets = append(bullets, "읽지 않은 검증 실패 신호 없음")
		}
		return Brief{
			Headline:  fmt.Sprintf("오늘 확인할 변경 %d건", len(groups)),
			Bullets:   bullets,
			Source:    "rule_based",
			AIHint:    "rule-based",
			CreatedAt: now,
		}
	}
	bullets := []string{
		fmt.Sprintf("Top group: %s", top.CommitSummary),
		fmt.Sprintf("%d groups, %d revisions, %d artifacts", len(groups), totalRevs, totalArtifacts),
	}
	if verificationRisk {
		bullets = append(bullets, "Includes changes that need verification")
	} else {
		bullets = append(bullets, "No unread verification failures")
	}
	return Brief{
		Headline:  fmt.Sprintf("%d changes to review today", len(groups)),
		Bullets:   bullets,
		Source:    "rule_based",
		AIHint:    "rule-based",
		CreatedAt: now,
	}
}

func SummaryCacheKey(accountID, projectSlug, userID string, baselineRevisionID, maxRevisionID int, locale, filterHash string) string {
	h := sha256.Sum256([]byte(strings.Join([]string{
		accountID,
		projectSlug,
		userID,
		fmt.Sprint(baselineRevisionID),
		fmt.Sprint(maxRevisionID),
		locale,
		filterHash,
	}, "\x00")))
	return hex.EncodeToString(h[:])
}

type LLMGroup struct {
	GroupID       string     `json:"group_id"`
	Kind          string     `json:"kind"`
	CommitSummary string     `json:"commit_summary"`
	Time          time.Time  `json:"time"`
	ArtifactCount int        `json:"artifact_count"`
	Areas         []string   `json:"areas"`
	Importance    Importance `json:"importance"`
}

func CompactLLMInput(groups []Group, cap int) []LLMGroup {
	if cap <= 0 || cap > len(groups) {
		cap = len(groups)
	}
	out := make([]LLMGroup, 0, cap)
	for _, g := range groups[:cap] {
		out = append(out, LLMGroup{
			GroupID:       g.GroupID,
			Kind:          g.GroupKind,
			CommitSummary: g.CommitSummary,
			Time:          g.TimeEnd,
			ArtifactCount: g.ArtifactCount,
			Areas:         append([]string{}, g.Areas...),
			Importance:    g.Importance,
		})
	}
	return out
}

func SourceBoundPrompt(groups []Group, locale string, cap int) string {
	payload, _ := json.Marshal(CompactLLMInput(groups, cap))
	return strings.TrimSpace(fmt.Sprintf(`
You summarize only the supplied Pindoc Change Group metadata.
Do not invent files, artifacts, users, or outcomes that are not present.
Return strict JSON: {"headline": "...", "bullets": ["...", "...", "..."]}.
Locale: %s
ChangeGroups: %s
`, locale, payload))
}

func syntheticGroupID(scope, keyKind, keyValue string, windowStart time.Time) string {
	h := sha256.Sum256([]byte(strings.Join([]string{
		scope,
		keyKind,
		keyValue,
		windowStart.UTC().Format(time.RFC3339),
	}, "\x00")))
	return "chg_" + hex.EncodeToString(h[:])[:16]
}

type groupAcc struct {
	Group
	artifactSet map[string]struct{}
	areaSet     map[string]struct{}
	authorSet   map[string]struct{}
	commitMsgs  []string
	commitSet   map[string]struct{}
	kindCounts  map[string]int
	verifySet   map[string]int
}

func newGroupAcc(groupID string, key GroupingKey, createdAt time.Time) *groupAcc {
	return &groupAcc{
		Group: Group{
			GroupID:     groupID,
			GroupingKey: key,
			TimeStart:   createdAt,
			TimeEnd:     createdAt,
		},
		artifactSet: map[string]struct{}{},
		areaSet:     map[string]struct{}{},
		authorSet:   map[string]struct{}{},
		commitSet:   map[string]struct{}{},
		kindCounts:  map[string]int{},
		verifySet:   map[string]int{},
	}
}

func (a *groupAcc) add(row RevisionRow) {
	a.RevisionCount++
	if row.CreatedAt.Before(a.TimeStart) {
		a.TimeStart = row.CreatedAt
	}
	if row.CreatedAt.After(a.TimeEnd) {
		a.TimeEnd = row.CreatedAt
	}
	if row.ArtifactID != "" {
		a.artifactSet[row.ArtifactID] = struct{}{}
	} else if row.ArtifactSlug != "" {
		a.artifactSet[row.ArtifactSlug] = struct{}{}
	}
	if row.AreaSlug != "" {
		a.areaSet[row.AreaSlug] = struct{}{}
	}
	if row.AuthorID != "" {
		a.authorSet[row.AuthorID] = struct{}{}
	}
	msg := strings.TrimSpace(row.CommitMsg)
	if msg != "" {
		if _, ok := a.commitSet[msg]; !ok {
			a.commitSet[msg] = struct{}{}
			a.commitMsgs = append(a.commitMsgs, msg)
		}
	}
	a.kindCounts[classifyGroupKind(row)]++
	if state := artifactVerificationState(row.ArtifactMeta); state != "" {
		a.verifySet[state]++
	}
}

func (a *groupAcc) finish() Group {
	a.Areas = sortedKeys(a.areaSet)
	a.Authors = sortedKeys(a.authorSet)
	a.ArtifactCount = len(a.artifactSet)
	a.GroupKind = dominantKind(a.kindCounts)
	a.VerificationState = aggregateVerification(a.verifySet)
	a.CommitSummary = summarizeCommits(a.commitMsgs, a.RevisionCount, a.ArtifactCount)
	a.Importance = buildImportance(a.GroupKind, a.ArtifactCount, a.VerificationState)
	return a.Group
}

func groupingKey(row RevisionRow, window time.Duration) (GroupingKey, time.Time) {
	ref := parseSourceSession(row.SourceSessionRef)
	windowStart := row.CreatedAt.Truncate(window)
	if ref.BulkOpID != "" {
		return GroupingKey{Kind: "bulk_op_id", Value: ref.BulkOpID, Confidence: "high"}, time.Time{}
	}
	if ref.SourceSession != "" && (ref.TurnID != "" || ref.RunID != "") {
		id := firstNonEmpty(ref.TurnID, ref.RunID)
		return GroupingKey{Kind: "source_session_turn", Value: ref.SourceSession + ":" + id, Confidence: "high"}, time.Time{}
	}
	if ref.TaskID != "" || ref.AgentRunID != "" {
		id := firstNonEmpty(ref.TaskID, ref.AgentRunID)
		return GroupingKey{Kind: "task_or_agent_run", Value: id, Confidence: "medium"}, windowStart
	}
	if ref.SourceSession != "" {
		return GroupingKey{Kind: "source_session_time_window", Value: ref.SourceSession, Confidence: "medium"}, windowStart
	}
	author := row.AuthorID
	if author == "" {
		author = "unknown"
	}
	return GroupingKey{Kind: "author_time_window", Value: author, Confidence: "low"}, windowStart
}

type sourceSession struct {
	AgentID       string
	SourceSession string
	BulkOpID      string
	TurnID        string
	RunID         string
	TaskID        string
	AgentRunID    string
}

func parseSourceSession(raw json.RawMessage) sourceSession {
	out := sourceSession{}
	if len(raw) == 0 {
		return out
	}
	values := map[string]any{}
	if err := json.Unmarshal(raw, &values); err != nil {
		return out
	}
	get := func(keys ...string) string {
		for _, key := range keys {
			if v, ok := values[key]; ok {
				switch t := v.(type) {
				case string:
					if strings.TrimSpace(t) != "" {
						return strings.TrimSpace(t)
					}
				case float64:
					return fmt.Sprintf("%.0f", t)
				}
			}
		}
		return ""
	}
	out.AgentID = get("agent_id")
	out.SourceSession = get("source_session", "session_id")
	out.BulkOpID = get("bulk_op_id")
	out.TurnID = get("turn_id", "conversation_turn_id")
	out.RunID = get("run_id")
	out.TaskID = get("task_id")
	out.AgentRunID = get("agent_run_id")
	return out
}

func artifactVerificationState(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var meta struct {
		VerificationState string `json:"verification_state"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.VerificationState)
}

func classifyGroupKind(row RevisionRow) string {
	lower := strings.ToLower(strings.Join([]string{row.CommitMsg, row.RevisionShape, string(row.SourceSessionRef)}, " "))
	switch {
	case strings.Contains(lower, "system_auto") || strings.Contains(lower, `"agent_id":"system"`) || strings.Contains(lower, `"agent_id": "system"`):
		return "system"
	case containsAny(lower, "maintenance", "migration", "migrate", "cleanup", "seed", "daemon", "docker", "health", "service"):
		return "maintenance"
	case containsAny(lower, "sync", "backfill", "reconcile", "embed", "harness", "import"):
		return "auto_sync"
	default:
		return "human_trigger"
	}
}

func buildImportance(kind string, artifactCount int, verification string) Importance {
	out := Importance{Level: "low"}
	if kind == "human_trigger" {
		out.Score++
		out.Reasons = append(out.Reasons, "human_trigger")
	}
	if artifactCount >= 2 {
		out.Score++
		out.Reasons = append(out.Reasons, "multi_artifact")
	}
	if verification == "unverified" || verification == "partially_verified" {
		out.Score++
		out.Reasons = append(out.Reasons, "verification_attention")
	}
	switch {
	case out.Score >= 2:
		out.Level = "high"
	case out.Score == 1:
		out.Level = "medium"
	default:
		out.Level = "low"
	}
	return out
}

func aggregateVerification(counts map[string]int) string {
	if len(counts) == 0 {
		return "unclassified"
	}
	if counts["unverified"] > 0 {
		return "unverified"
	}
	if counts["partially_verified"] > 0 {
		return "partially_verified"
	}
	if counts["verified"] > 0 {
		return "verified"
	}
	return "unclassified"
}

func dominantKind(counts map[string]int) string {
	order := []string{"human_trigger", "auto_sync", "maintenance", "system"}
	best := "human_trigger"
	bestCount := -1
	for _, kind := range order {
		if counts[kind] > bestCount {
			best = kind
			bestCount = counts[kind]
		}
	}
	return best
}

func summarizeCommits(msgs []string, revisionCount, artifactCount int) string {
	if len(msgs) == 0 {
		return fmt.Sprintf("%d revision(s) across %d artifact(s)", revisionCount, artifactCount)
	}
	trimmed := make([]string, 0, min(len(msgs), 3))
	for _, msg := range msgs {
		trimmed = append(trimmed, trimText(msg, 90))
		if len(trimmed) == 3 {
			break
		}
	}
	summary := strings.Join(trimmed, "; ")
	if len(msgs) > len(trimmed) {
		summary += fmt.Sprintf(" (+%d more)", len(msgs)-len(trimmed))
	}
	return summary
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func trimText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		if s == "" {
			return ""
		}
		return "..."
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return strings.TrimSpace(string(runes[:max])) + "..."
}
