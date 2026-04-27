package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/var-gg/pindoc/internal/pindoc/changegroup"
	"github.com/var-gg/pindoc/internal/pindoc/projectexport"
)

type changeGroupsResponse struct {
	ProjectSlug   string               `json:"project_slug"`
	Groups        []changegroup.Group  `json:"groups"`
	Summary       changegroup.Brief    `json:"summary"`
	Baseline      changegroup.Baseline `json:"baseline"`
	MaxRevisionID int                  `json:"max_revision_id"`
}

func (d Deps) handleChangeGroups(w http.ResponseWriter, r *http.Request) {
	projectSlug := projectSlugFrom(r)
	q := r.URL.Query()
	limit := intParam(q.Get("limit"), 30)
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	locale := q.Get("locale")
	if locale == "" {
		locale = d.DefaultProjectLocale
	}
	if locale == "" {
		locale = "en"
	}
	userKey := readerUserKey(r)
	maxRevisionID, err := changegroup.ProjectRevisionWatermark(r.Context(), d.DB, projectSlug)
	if err != nil {
		d.Logger.Error("change groups watermark", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	baseline, sinceTime, err := d.changeGroupBaseline(r, projectSlug, userKey)
	if err != nil {
		d.Logger.Error("change groups baseline", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	opts := changegroup.Options{
		Limit:           limit,
		AreaSlug:        q.Get("area"),
		Kind:            q.Get("kind"),
		SinceRevisionID: intParam(q.Get("since_revision_id"), 0),
		SinceTime:       sinceTime,
	}
	groups, err := changegroup.Query(r.Context(), d.DB, projectSlug, opts)
	if err != nil {
		d.Logger.Error("change groups query", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	// Flow 1a 3-tier fallback (docs/06-ui-flows.md): if the watermark-since
	// query is empty, retry with last 7 days; if that's still empty, drop
	// the time filter and let importance ordering surface the top groups.
	// since_revision_id callers are explicit about their cutoff and bypass
	// fallback entirely.
	if len(groups) == 0 && opts.SinceRevisionID == 0 {
		if baseline.LastSeenAt != nil {
			cutoff := time.Now().Add(-7 * 24 * time.Hour)
			retry := opts
			retry.SinceTime = &cutoff
			groups, err = changegroup.Query(r.Context(), d.DB, projectSlug, retry)
			if err != nil {
				d.Logger.Error("change groups 7d fallback query", "err", err)
				writeError(w, http.StatusInternalServerError, "query failed")
				return
			}
			if len(groups) > 0 {
				baseline.FallbackUsed = "recent_7d"
			}
		}
		if len(groups) == 0 {
			retry := opts
			retry.SinceTime = nil
			groups, err = changegroup.Query(r.Context(), d.DB, projectSlug, retry)
			if err != nil {
				d.Logger.Error("change groups importance fallback query", "err", err)
				writeError(w, http.StatusInternalServerError, "query failed")
				return
			}
			if len(groups) > 0 {
				baseline.FallbackUsed = "importance_top"
			}
		}
	}
	summary, err := d.todaySummary(r, projectSlug, userKey, locale, baseline.RevisionWatermark, maxRevisionID, filterHash(q), groups)
	if err != nil {
		d.Logger.Warn("today summary cache failed; using template", "err", err)
		summary = changegroup.BuildTemplateBrief(groups, locale)
	}
	writeJSON(w, http.StatusOK, changeGroupsResponse{
		ProjectSlug:   projectSlug,
		Groups:        groups,
		Summary:       summary,
		Baseline:      baseline,
		MaxRevisionID: maxRevisionID,
	})
}

func (d Deps) changeGroupBaseline(r *http.Request, projectSlug, userKey string) (changegroup.Baseline, *time.Time, error) {
	var baseline changegroup.Baseline
	var seenAt time.Time
	err := d.DB.QueryRow(r.Context(), `
		SELECT rw.revision_watermark, rw.seen_at
		FROM reader_watermarks rw
		JOIN projects p ON p.id = rw.project_id
		WHERE p.slug = $1 AND rw.user_key = $2
	`, projectSlug, userKey).Scan(&baseline.RevisionWatermark, &seenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		cutoff := time.Now().Add(-7 * 24 * time.Hour)
		baseline.DefaultedToDays = 7
		return baseline, &cutoff, nil
	}
	if err != nil {
		return baseline, nil, err
	}
	baseline.LastSeenAt = &seenAt
	return baseline, &seenAt, nil
}

func (d Deps) handleReadMark(w http.ResponseWriter, r *http.Request) {
	projectSlug := projectSlugFrom(r)
	userKey := readerUserKey(r)
	var input struct {
		RevisionWatermark int `json:"revision_watermark"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}
	if input.RevisionWatermark <= 0 {
		watermark, err := changegroup.ProjectRevisionWatermark(r.Context(), d.DB, projectSlug)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "query failed")
			return
		}
		input.RevisionWatermark = watermark
	}
	if err := d.upsertReadWatermark(r, projectSlug, userKey, input.RevisionWatermark); err != nil {
		d.Logger.Error("read mark", "err", err)
		writeError(w, http.StatusInternalServerError, "write failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_slug":       projectSlug,
		"user_key":           userKey,
		"revision_watermark": input.RevisionWatermark,
	})
}

func (d Deps) markCurrentProjectRevision(r *http.Request, projectSlug, userKey string) error {
	watermark, err := changegroup.ProjectRevisionWatermark(r.Context(), d.DB, projectSlug)
	if err != nil {
		return err
	}
	if watermark <= 0 {
		return nil
	}
	return d.upsertReadWatermark(r, projectSlug, userKey, watermark)
}

func (d Deps) upsertReadWatermark(r *http.Request, projectSlug, userKey string, revisionWatermark int) error {
	_, err := d.DB.Exec(r.Context(), `
		INSERT INTO reader_watermarks (user_key, project_id, revision_watermark, seen_at)
		SELECT $2, p.id, $3, now()
		FROM projects p
		WHERE p.slug = $1
		ON CONFLICT (user_key, project_id)
		DO UPDATE SET revision_watermark = EXCLUDED.revision_watermark, seen_at = now()
	`, projectSlug, userKey, revisionWatermark)
	return err
}

func (d Deps) todaySummary(r *http.Request, projectSlug, userKey, locale string, baselineRev, maxRev int, filterHash string, groups []changegroup.Group) (changegroup.Brief, error) {
	cacheKey := changegroup.SummaryCacheKey("local", projectSlug, userKey, baselineRev, maxRev, locale, filterHash)
	var brief changegroup.Brief
	var bullets []string
	err := d.DB.QueryRow(r.Context(), `
		SELECT headline, bullets, source, created_at
		FROM summary_cache
		WHERE cache_key = $1
		  AND (expires_at IS NULL OR expires_at > now())
	`, cacheKey).Scan(&brief.Headline, &bullets, &brief.Source, &brief.CreatedAt)
	if err == nil {
		brief.Bullets = bullets
		if brief.Source == "llm" {
			brief.AIHint = "AI-generated"
		} else {
			brief.AIHint = "rule-based"
		}
		return brief, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return brief, err
	}

	brief = changegroup.BuildTemplateBrief(groups, locale)
	tokenEstimate := changegroup.EstimateSummaryTokens(groups, d.Summary.GroupCap)
	if strings.TrimSpace(d.Summary.Endpoint) != "" && d.underDailySummaryCap(r, projectSlug, userKey, tokenEstimate) {
		if llmBrief, used, err := changegroup.RequestLLMBrief(r.Context(), d.Summary, groups, locale); err == nil {
			brief = llmBrief
			tokenEstimate = used
			d.addDailySummaryUsage(r, projectSlug, userKey, tokenEstimate)
		} else if d.Logger != nil {
			d.Logger.Warn("summary LLM failed; using rule template", "err", err)
		}
	}
	inputHash := shaHex(changegroup.SourceBoundPrompt(groups, locale, d.Summary.GroupCap))
	_, err = d.DB.Exec(r.Context(), `
		INSERT INTO summary_cache (
			cache_key, project_id, user_key, locale, filter_hash,
			baseline_revision_id, max_revision_id, headline, bullets,
			source, input_hash, token_estimate, expires_at
		)
		SELECT $1, p.id, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now() + interval '12 hours'
		FROM projects p
		WHERE p.slug = $12
		ON CONFLICT (cache_key) DO NOTHING
	`, cacheKey, userKey, locale, filterHash, baselineRev, maxRev, brief.Headline, brief.Bullets, brief.Source, inputHash, tokenEstimate, projectSlug)
	return brief, err
}

func (d Deps) underDailySummaryCap(r *http.Request, projectSlug, userKey string, tokenEstimate int) bool {
	if d.Summary.DailyTokenCap <= 0 {
		return true
	}
	var used int
	err := d.DB.QueryRow(r.Context(), `
		SELECT COALESCE(sud.tokens_used, 0)
		FROM projects p
		LEFT JOIN summary_usage_daily sud
		  ON sud.project_id = p.id
		 AND sud.user_key = $2
		 AND sud.day = CURRENT_DATE
		WHERE p.slug = $1
	`, projectSlug, userKey).Scan(&used)
	if err != nil {
		return false
	}
	return used+tokenEstimate <= d.Summary.DailyTokenCap
}

func (d Deps) addDailySummaryUsage(r *http.Request, projectSlug, userKey string, tokenEstimate int) {
	_, err := d.DB.Exec(r.Context(), `
		INSERT INTO summary_usage_daily (user_key, project_id, day, tokens_used)
		SELECT $2, p.id, CURRENT_DATE, $3
		FROM projects p
		WHERE p.slug = $1
		ON CONFLICT (user_key, project_id, day)
		DO UPDATE SET tokens_used = summary_usage_daily.tokens_used + EXCLUDED.tokens_used
	`, projectSlug, userKey, tokenEstimate)
	if err != nil && d.Logger != nil {
		d.Logger.Warn("summary usage update failed", "err", err)
	}
}

func (d Deps) handleProjectExport(w http.ResponseWriter, r *http.Request) {
	projectSlug := projectSlugFrom(r)
	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "zip"
	}
	archive, err := projectexport.BuildFromDB(r.Context(), d.DB, projectexport.Options{
		ProjectSlug:      projectSlug,
		Areas:            splitList(q.Get("area")),
		Slugs:            splitList(q.Get("slug")),
		IncludeRevisions: q.Get("include_revisions") == "true",
		Format:           format,
	})
	if err != nil {
		d.Logger.Error("project export", "err", err)
		writeError(w, http.StatusInternalServerError, "export failed")
		return
	}
	w.Header().Set("Content-Type", archive.MimeType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+archive.Filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(archive.Bytes)
}

func readerUserKey(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Pindoc-User")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.URL.Query().Get("user_key")); v != "" {
		return v
	}
	return "local"
}

func intParam(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func filterHash(q map[string][]string) string {
	parts := []string{}
	for _, key := range []string{"area", "kind", "limit", "since_revision_id"} {
		if values, ok := q[key]; ok {
			parts = append(parts, key+"="+strings.Join(values, ","))
		}
	}
	return shaHex(strings.Join(parts, "&"))[:16]
}

func shaHex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
