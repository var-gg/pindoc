package tools

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/var-gg/pindoc/internal/pindoc/auth"
	pgit "github.com/var-gg/pindoc/internal/pindoc/git"
	pinmodel "github.com/var-gg/pindoc/internal/pindoc/pins"
)

const defaultPinCandidatesTopN = 5

type PinCandidatesAttention struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	NextTools []NextToolHint `json:"next_tools,omitempty"`
	Level     string         `json:"level"`
	TopN      int            `json:"top_n"`
	Count     int            `json:"count"`
	Items     []PinCandidate `json:"items"`
}

type PinCandidate struct {
	RepoID        string `json:"repo_id"`
	CommitSHA     string `json:"commit_sha"`
	Path          string `json:"path"`
	LinesStart    *int   `json:"lines_start"`
	LinesEnd      *int   `json:"lines_end"`
	KindInferred  string `json:"kind_inferred"`
	Reason        string `json:"reason"`
	CommitSummary string `json:"commit_summary"`
}

type pinCandidateCoordinate struct {
	RepoID    string
	CommitSHA string
	Path      string
}

func buildPinCandidatesAttention(ctx context.Context, deps Deps, p *auth.Principal, scope *auth.ProjectScope, landings []ContextLanding) *PinCandidatesAttention {
	if callerAgentAssignee(p) == "" {
		return nil
	}
	topN := pinCandidatesTopN()
	authors := callerPinAuthorNeedles(ctx, deps, p)
	if len(authors) == 0 {
		return nil
	}
	repos, err := pgit.LoadProjectRepos(ctx, deps.DB, scope.ProjectID)
	if err != nil || len(repos) == 0 {
		if err != nil && deps.Logger != nil {
			deps.Logger.Warn("context.for_task pin_candidates repo load failed", "err", err)
		}
		return nil
	}
	existing, err := pinnedCoordinatesForLandings(ctx, deps, landings)
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Warn("context.for_task pin_candidates dedupe load failed", "err", err)
		}
		existing = nil
	}

	provider := pgit.LocalGitProvider{}
	seen := map[string]bool{}
	var candidates []PinCandidate
	for _, repo := range repos {
		if !provider.Available(ctx, repo) {
			continue
		}
		for _, author := range authors {
			commits, err := provider.RecentCommitFiles(ctx, repo, author, topN)
			if err != nil {
				if deps.Logger != nil {
					deps.Logger.Warn("context.for_task pin_candidates git log failed", "repo_id", repo.ID, "author", author, "err", err)
				}
				continue
			}
			for commitIdx, commit := range commits {
				for _, file := range commit.Files {
					if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(file.Status)), "D") {
						continue
					}
					path := strings.TrimSpace(file.Path)
					if path == "" {
						continue
					}
					c := PinCandidate{
						RepoID:        repo.ID,
						CommitSHA:     strings.TrimSpace(commit.SHA),
						Path:          path,
						LinesStart:    nil,
						LinesEnd:      nil,
						KindInferred:  pinmodel.NormalizeKind("", path),
						Reason:        pinCandidateReason(commitIdx, author),
						CommitSummary: strings.TrimSpace(commit.Summary),
					}
					if coordinatePinned(existing, c) {
						continue
					}
					key := strings.ToLower(c.RepoID) + "\x00" + c.CommitSHA + "\x00" + c.Path
					if seen[key] {
						continue
					}
					seen[key] = true
					candidates = append(candidates, c)
					if len(candidates) >= topN {
						return pinCandidatesEnvelope(scope.ProjectSlug, candidates, topN)
					}
				}
			}
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	return pinCandidatesEnvelope(scope.ProjectSlug, candidates, topN)
}

func pinCandidatesEnvelope(projectSlug string, items []PinCandidate, topN int) *PinCandidatesAttention {
	return &PinCandidatesAttention{
		Code:    "pin_candidates_available",
		Message: fmt.Sprintf("Recent matching local Git commits produced %d pin candidate(s).", len(items)),
		Level:   "info",
		TopN:    topN,
		Count:   len(items),
		Items:   items,
		NextTools: []NextToolHint{
			{
				Tool: "pindoc.artifact.propose",
				Args: map[string]any{
					"project_slug": projectSlug,
					"pins":         "use selected pin_candidates.items entries",
				},
				Reason: "attach selected candidates while creating or updating an artifact",
			},
			{
				Tool: "pindoc.artifact.add_pin",
				Args: map[string]any{
					"project_slug": projectSlug,
					"pin":          "use one selected pin_candidates.items entry",
				},
				Reason: "attach one candidate after the artifact already exists",
			},
		},
	}
}

func pinCandidatesTopN() int {
	raw := strings.TrimSpace(os.Getenv("PINDOC_PIN_CANDIDATES_TOP_N"))
	if raw == "" {
		return defaultPinCandidatesTopN
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultPinCandidatesTopN
	}
	return n
}

func callerPinAuthorNeedles(ctx context.Context, deps Deps, p *auth.Principal) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		key := strings.ToLower(s)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}
	if p != nil && strings.TrimSpace(p.UserID) != "" {
		var email, displayName sql.NullString
		if err := deps.DB.QueryRow(ctx, `
			SELECT email, display_name
			  FROM users
			 WHERE id = $1::uuid
			   AND deleted_at IS NULL
			 LIMIT 1
		`, strings.TrimSpace(p.UserID)).Scan(&email, &displayName); err == nil {
			if email.Valid {
				add(email.String)
			}
			if displayName.Valid {
				add(displayName.String)
			}
		}
	}
	if p != nil {
		add(stripAgentPrefix(p.AgentID))
	}
	return out
}

func pinnedCoordinatesForLandings(ctx context.Context, deps Deps, landings []ContextLanding) ([]pinCandidateCoordinate, error) {
	if len(landings) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(landings))
	for _, landing := range landings {
		if strings.TrimSpace(landing.ArtifactID) != "" {
			ids = append(ids, landing.ArtifactID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := deps.DB.Query(ctx, `
		SELECT COALESCE(repo_id::text, ''), COALESCE(commit_sha, ''), path
		  FROM artifact_pins
		 WHERE artifact_id = ANY($1::uuid[])
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pinCandidateCoordinate
	for rows.Next() {
		var c pinCandidateCoordinate
		if err := rows.Scan(&c.RepoID, &c.CommitSHA, &c.Path); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func coordinatePinned(existing []pinCandidateCoordinate, candidate PinCandidate) bool {
	for _, c := range existing {
		if strings.TrimSpace(c.Path) != strings.TrimSpace(candidate.Path) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(c.RepoID), strings.TrimSpace(candidate.RepoID)) {
			continue
		}
		if commitsEquivalent(c.CommitSHA, candidate.CommitSHA) {
			return true
		}
	}
	return false
}

func commitsEquivalent(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return a == b
	}
	return a == b || strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

func pinCandidateReason(commitIndex int, author string) string {
	author = strings.TrimSpace(author)
	if commitIndex == 0 {
		return "caller-matching most recent local Git commit for author " + author
	}
	return fmt.Sprintf("caller-matching local Git commit HEAD~%d for author %s", commitIndex, author)
}
