package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/var-gg/pindoc/internal/pindoc/artifact/preflight"
	"github.com/var-gg/pindoc/internal/pindoc/db"
)

type acceptanceVerbReportRow struct {
	ProjectSlug string
	TaskSlug    string
	Title       string
	Revision    int
	Finding     preflight.AcceptanceVerbFinding
}

func loadAcceptanceVerbFindings(ctx context.Context, pool *db.Pool, projects []string, limit int) ([]acceptanceVerbReportRow, error) {
	args := []any{projects}
	limitSQL := ""
	if limit > 0 {
		args = append(args, limit)
		limitSQL = fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := pool.Query(ctx, `
		SELECT p.slug, a.slug, a.title, a.body_markdown,
		       COALESCE((SELECT max(revision_number) FROM artifact_revisions WHERE artifact_id = a.id), 0)
		  FROM artifacts a
		  JOIN projects p ON p.id = a.project_id
		 WHERE a.type = 'Task'
		   AND a.status <> 'archived'
		   AND a.status <> 'superseded'
		   AND NOT starts_with(a.slug, '_template_')
		   AND (cardinality($1::text[]) = 0 OR p.slug = ANY($1::text[]))
		 ORDER BY p.slug, a.slug`+limitSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []acceptanceVerbReportRow
	for rows.Next() {
		var projectSlug, taskSlug, title, body string
		var revision int
		if err := rows.Scan(&projectSlug, &taskSlug, &title, &body, &revision); err != nil {
			return nil, err
		}
		for _, finding := range preflight.LintAcceptanceVerbs(body) {
			out = append(out, acceptanceVerbReportRow{
				ProjectSlug: projectSlug,
				TaskSlug:    taskSlug,
				Title:       title,
				Revision:    revision,
				Finding:     finding,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ProjectSlug != out[j].ProjectSlug {
			return out[i].ProjectSlug < out[j].ProjectSlug
		}
		if out[i].TaskSlug != out[j].TaskSlug {
			return out[i].TaskSlug < out[j].TaskSlug
		}
		return out[i].Finding.LineNumber < out[j].Finding.LineNumber
	})
	return out, nil
}

func writeAcceptanceVerbReport(path string, findings []acceptanceVerbReportRow) error {
	body := renderAcceptanceVerbReport(findings)
	if strings.TrimSpace(path) == "" || path == "-" {
		_, err := fmt.Fprint(os.Stdout, body)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func printAcceptanceVerbReportSummary(path string, findings []acceptanceVerbReportRow) {
	target := path
	if strings.TrimSpace(target) == "" {
		target = "stdout"
	}
	fmt.Fprintf(os.Stdout, "acceptance_verb_lint findings=%d report=%s\n", len(findings), target)
}

func renderAcceptanceVerbReport(findings []acceptanceVerbReportRow) string {
	var b strings.Builder
	b.WriteString("# Acceptance Verb Lint Retro-pass Report\n\n")
	b.WriteString(fmt.Sprintf("Total findings: %d\n\n", len(findings)))
	if len(findings) == 0 {
		b.WriteString("No forbidden action verbs found in Task acceptance checklists.\n")
		return b.String()
	}
	b.WriteString("| Project | Task | Rev | Line | Checkbox | Verb | Criterion | Suggested outcome |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | --- | --- | --- |\n")
	for _, row := range findings {
		b.WriteString(fmt.Sprintf(
			"| %s | `%s` | %d | %d | %d | %s | %s | %s |\n",
			mdCell(row.ProjectSlug),
			mdCell(row.TaskSlug),
			row.Revision,
			row.Finding.LineNumber,
			row.Finding.CheckboxIndex,
			mdCell(row.Finding.Verb),
			mdCell(row.Finding.Text),
			mdCell(row.Finding.ExampleAfter),
		))
	}
	return b.String()
}

func mdCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}
