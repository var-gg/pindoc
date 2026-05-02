package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/var-gg/pindoc/internal/pindoc/artifact/preflight"
	"github.com/var-gg/pindoc/internal/pindoc/db"
)

type outcomeMissingReportRow struct {
	ProjectSlug string
	TaskSlug    string
	Title       string
	Revision    int
	Codes       []string
	Prompt      string
}

func loadOutcomeMissingFindings(ctx context.Context, pool *db.Pool, projects []string, limit int) ([]outcomeMissingReportRow, error) {
	args := []any{projects}
	limitSQL := ""
	if limit > 0 {
		args = append(args, limit)
		limitSQL = fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := pool.Query(ctx, `
		SELECT p.slug, a.slug, a.title, a.body_markdown,
		       COALESCE(a.task_meta, '{}'::jsonb)::text,
		       COALESCE(a.artifact_meta, '{}'::jsonb)::text,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		  FROM artifacts a
		  JOIN projects p ON p.id = a.project_id
		 WHERE a.type = 'Task'
		   AND a.status <> 'archived'
		   AND a.status <> 'superseded'
		   AND COALESCE(NULLIF(a.task_meta->>'status', ''), 'open') = 'claimed_done'
		   AND NOT starts_with(a.slug, '_template_')
		   AND (cardinality($1::text[]) = 0 OR p.slug = ANY($1::text[]))
		 ORDER BY p.slug, a.slug`+limitSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []outcomeMissingReportRow
	for rows.Next() {
		var projectSlug, taskSlug, title, body string
		var taskMetaRaw, artifactMetaRaw []byte
		var revision int
		if err := rows.Scan(&projectSlug, &taskSlug, &title, &body, &taskMetaRaw, &artifactMetaRaw, &revision); err != nil {
			return nil, err
		}
		commitRequired := !outcomeCommitExemptForReport(taskMetaRaw, artifactMetaRaw)
		result := preflight.CheckOutcomeSection(body, preflight.OutcomeCheckOptions{CommitRequired: commitRequired})
		if result.OK() {
			continue
		}
		out = append(out, outcomeMissingReportRow{
			ProjectSlug: projectSlug,
			TaskSlug:    taskSlug,
			Title:       title,
			Revision:    revision,
			Codes:       result.Codes,
			Prompt:      preflight.ReverseEngineerOutcomePrompt(projectSlug, taskSlug, result.Codes),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ProjectSlug != out[j].ProjectSlug {
			return out[i].ProjectSlug < out[j].ProjectSlug
		}
		return out[i].TaskSlug < out[j].TaskSlug
	})
	return out, nil
}

func writeOutcomeMissingReport(path string, rows []outcomeMissingReportRow) error {
	body := renderOutcomeMissingReport(rows)
	if strings.TrimSpace(path) == "" || path == "-" {
		_, err := fmt.Fprint(os.Stdout, body)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func printOutcomeMissingReportSummary(path string, rows []outcomeMissingReportRow) {
	target := path
	if strings.TrimSpace(target) == "" {
		target = "stdout"
	}
	fmt.Fprintf(os.Stdout, "claim_done_outcome_missing findings=%d report=%s\n", len(rows), target)
}

func renderOutcomeMissingReport(rows []outcomeMissingReportRow) string {
	var b strings.Builder
	b.WriteString("# claim_done Outcome Missing Retro-pass Report\n\n")
	b.WriteString(fmt.Sprintf("Total findings: %d\n\n", len(rows)))
	if len(rows) == 0 {
		b.WriteString("No claimed_done Tasks are missing required Outcome content.\n")
		return b.String()
	}
	b.WriteString("| Project | Task | Rev | Missing checks | Reverse-engineer prompt |\n")
	b.WriteString("| --- | --- | ---: | --- | --- |\n")
	for _, row := range rows {
		b.WriteString(fmt.Sprintf(
			"| %s | `%s` | %d | %s | %s |\n",
			mdCell(row.ProjectSlug),
			mdCell(row.TaskSlug),
			row.Revision,
			mdCell(strings.Join(row.Codes, ", ")),
			mdCell(row.Prompt),
		))
	}
	return b.String()
}

func outcomeCommitExemptForReport(taskMetaRaw, artifactMetaRaw []byte) bool {
	return jsonMetaBool(taskMetaRaw, "outcome_commit_exempt") ||
		jsonMetaBool(taskMetaRaw, "code_coordinate_exempt") ||
		jsonMetaBool(artifactMetaRaw, "outcome_commit_exempt") ||
		jsonMetaBool(artifactMetaRaw, "code_coordinate_exempt")
}

func jsonMetaBool(raw []byte, key string) bool {
	if len(raw) == 0 {
		return false
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return false
	}
	v, ok := meta[key].(bool)
	return ok && v
}
