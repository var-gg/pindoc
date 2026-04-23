package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// TelemetryToolRow is one row in the aggregated telemetry table — one
// tool's behaviour over the selected window. Token totals are
// approximations via tiktoken cl100k_base (see telemetry package).
type TelemetryToolRow struct {
	ToolName          string  `json:"tool_name"`
	Calls             int     `json:"calls"`
	Errors            int     `json:"errors"`
	ErrorRate         float64 `json:"error_rate"`
	AvgDurationMs     int     `json:"avg_duration_ms"`
	P50DurationMs     int     `json:"p50_duration_ms"`
	P95DurationMs     int     `json:"p95_duration_ms"`
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	AvgInputTokens    int     `json:"avg_input_tokens"`
	AvgOutputTokens   int     `json:"avg_output_tokens"`
	AvgInputBytes     int     `json:"avg_input_bytes"`
	AvgOutputBytes    int     `json:"avg_output_bytes"`
	LastCallAt        string  `json:"last_call_at"`
}

// TelemetryRecentCall is one row in the "recent calls" drill-down —
// the raw per-call timeline agents scan to understand a spike.
type TelemetryRecentCall struct {
	StartedAt       string `json:"started_at"`
	DurationMs     int    `json:"duration_ms"`
	ToolName       string `json:"tool_name"`
	AgentID        string `json:"agent_id,omitempty"`
	ProjectSlug    string `json:"project_slug,omitempty"`
	InputBytes     int    `json:"input_bytes"`
	OutputBytes    int    `json:"output_bytes"`
	InputTokens    int    `json:"input_tokens_est"`
	OutputTokens   int    `json:"output_tokens_est"`
	ErrorCode      string `json:"error_code,omitempty"`
	ToolsetVersion string `json:"toolset_version,omitempty"`
}

// TelemetryResponse is the UI-facing payload. Per-tool aggregates plus
// a recent-calls sample and a totals band for the top of the page.
type TelemetryResponse struct {
	WindowHours int                   `json:"window_hours"`
	ProjectSlug string                `json:"project_slug,omitempty"`
	Totals      TelemetryTotals       `json:"totals"`
	Tools       []TelemetryToolRow    `json:"tools"`
	Recent      []TelemetryRecentCall `json:"recent"`
}

type TelemetryTotals struct {
	Calls             int `json:"calls"`
	Errors            int `json:"errors"`
	TotalInputTokens  int `json:"total_input_tokens"`
	TotalOutputTokens int `json:"total_output_tokens"`
	UniqueAgents      int `json:"unique_agents"`
}

// handleTelemetry aggregates mcp_tool_calls over a configurable time
// window (default 24h, max 720h = 30 days) and returns a UI-ready
// payload. Read-only: matches the rest of httpapi's "UI never writes"
// principle; writes stay on the MCP side.
//
// Query params:
//   - window: "1h" | "24h" | "7d" | "30d" (defaults to 24h)
//   - project: filter to one project_slug (optional; default = all)
//   - recent_limit: number of recent calls to include (default 50, max 500)
func (d Deps) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	windowHours := parseWindowHours(r.URL.Query().Get("window"))
	projectFilter := strings.TrimSpace(r.URL.Query().Get("project"))
	recentLimit := parseLimit(r.URL.Query().Get("recent_limit"), 50, 500)

	ctx := r.Context()
	cutoff := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)

	resp := TelemetryResponse{
		WindowHours: windowHours,
		ProjectSlug: projectFilter,
		Tools:       []TelemetryToolRow{},
		Recent:      []TelemetryRecentCall{},
	}

	// --- Per-tool aggregates ---
	// Column aliases are allowed in ORDER BY by themselves, but Postgres
	// can't resolve them inside an arithmetic expression there — it tries
	// to look them up as real column names. So the ORDER BY repeats the
	// sum expressions instead of referencing aliases.
	toolsSQL := `
		SELECT
			tool_name,
			count(*)                                        AS calls,
			count(*) FILTER (WHERE error_code IS NOT NULL)  AS errors,
			COALESCE(avg(duration_ms), 0)::int              AS avg_duration,
			COALESCE(percentile_disc(0.5) WITHIN GROUP (ORDER BY duration_ms), 0)  AS p50_duration,
			COALESCE(percentile_disc(0.95) WITHIN GROUP (ORDER BY duration_ms), 0) AS p95_duration,
			COALESCE(sum(input_tokens_est), 0)              AS total_in_tok,
			COALESCE(sum(output_tokens_est), 0)             AS total_out_tok,
			COALESCE(avg(input_tokens_est), 0)::int         AS avg_in_tok,
			COALESCE(avg(output_tokens_est), 0)::int        AS avg_out_tok,
			COALESCE(avg(input_bytes), 0)::int              AS avg_in_bytes,
			COALESCE(avg(output_bytes), 0)::int             AS avg_out_bytes,
			max(started_at)                                 AS last_call
		FROM mcp_tool_calls
		WHERE started_at >= $1
		  AND ($2::text IS NULL OR project_slug = $2)
		GROUP BY tool_name
		ORDER BY COALESCE(sum(input_tokens_est), 0) + COALESCE(sum(output_tokens_est), 0) DESC
	`
	var projectArg any
	if projectFilter != "" {
		projectArg = projectFilter
	}
	rows, err := d.DB.Query(ctx, toolsSQL, cutoff, projectArg)
	if err != nil {
		d.Logger.Error("telemetry tools query failed", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var row TelemetryToolRow
		var p50, p95 int
		var lastCall time.Time
		if err := rows.Scan(
			&row.ToolName, &row.Calls, &row.Errors, &row.AvgDurationMs,
			&p50, &p95,
			&row.TotalInputTokens, &row.TotalOutputTokens,
			&row.AvgInputTokens, &row.AvgOutputTokens,
			&row.AvgInputBytes, &row.AvgOutputBytes,
			&lastCall,
		); err != nil {
			d.Logger.Error("telemetry tools scan failed", "err", err)
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		row.P50DurationMs = p50
		row.P95DurationMs = p95
		if row.Calls > 0 {
			row.ErrorRate = float64(row.Errors) / float64(row.Calls)
		}
		row.LastCallAt = lastCall.Format(time.RFC3339)
		resp.Tools = append(resp.Tools, row)
	}
	if err := rows.Err(); err != nil {
		d.Logger.Error("telemetry tools iter failed", "err", err)
		writeError(w, http.StatusInternalServerError, "iter failed")
		return
	}

	// --- Totals band ---
	totalsSQL := `
		SELECT
			count(*),
			count(*) FILTER (WHERE error_code IS NOT NULL),
			COALESCE(sum(input_tokens_est), 0),
			COALESCE(sum(output_tokens_est), 0),
			count(DISTINCT agent_id) FILTER (WHERE agent_id IS NOT NULL)
		FROM mcp_tool_calls
		WHERE started_at >= $1
		  AND ($2::text IS NULL OR project_slug = $2)
	`
	if err := d.DB.QueryRow(ctx, totalsSQL, cutoff, projectArg).Scan(
		&resp.Totals.Calls, &resp.Totals.Errors,
		&resp.Totals.TotalInputTokens, &resp.Totals.TotalOutputTokens,
		&resp.Totals.UniqueAgents,
	); err != nil {
		d.Logger.Error("telemetry totals query failed", "err", err)
		writeError(w, http.StatusInternalServerError, "totals failed")
		return
	}

	// --- Recent calls ---
	recentSQL := `
		SELECT
			started_at, duration_ms, tool_name,
			COALESCE(agent_id, ''), COALESCE(project_slug, ''),
			input_bytes, output_bytes,
			input_tokens_est, output_tokens_est,
			COALESCE(error_code, ''), COALESCE(toolset_version, '')
		FROM mcp_tool_calls
		WHERE started_at >= $1
		  AND ($2::text IS NULL OR project_slug = $2)
		ORDER BY started_at DESC
		LIMIT $3
	`
	recentRows, err := d.DB.Query(ctx, recentSQL, cutoff, projectArg, recentLimit)
	if err != nil {
		d.Logger.Error("telemetry recent query failed", "err", err)
		writeError(w, http.StatusInternalServerError, "recent query failed")
		return
	}
	defer recentRows.Close()
	for recentRows.Next() {
		var rc TelemetryRecentCall
		var started time.Time
		if err := recentRows.Scan(
			&started, &rc.DurationMs, &rc.ToolName,
			&rc.AgentID, &rc.ProjectSlug,
			&rc.InputBytes, &rc.OutputBytes,
			&rc.InputTokens, &rc.OutputTokens,
			&rc.ErrorCode, &rc.ToolsetVersion,
		); err != nil {
			d.Logger.Error("telemetry recent scan failed", "err", err)
			continue
		}
		rc.StartedAt = started.Format(time.RFC3339)
		resp.Recent = append(resp.Recent, rc)
	}

	writeJSON(w, http.StatusOK, resp)
}

// parseWindowHours maps the client-facing string to a canonical hour
// count. Unknown values default to 24h. The 30-day ceiling matches the
// operator-facing "not a long-term analytics warehouse" scope.
func parseWindowHours(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1h":
		return 1
	case "6h":
		return 6
	case "24h", "":
		return 24
	case "7d":
		return 24 * 7
	case "30d":
		return 24 * 30
	}
	// Also accept raw hour integer for future flexibility.
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		if n > 24*30 {
			n = 24 * 30
		}
		return n
	}
	return 24
}

func parseLimit(s string, def, max int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
		if n > max {
			return max
		}
		return n
	}
	return def
}
