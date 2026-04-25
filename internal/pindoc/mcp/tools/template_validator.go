package tools

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
)

// validatorHints is the parsed result of a `<!-- validator: ... -->`
// comment at the top of a _template_* artifact body. The preflight
// layer in artifact_propose.go reads these hints per artifact.type so
// changing a template via update_of immediately re-anchors validator
// rules without a code change.
//
// Task `preflight-template-drift-통합`. Current axes:
//
//   required_h2       — H2 headings whose presence is checked by
//                       requiredH2Warnings (slash-mixed headings like
//                       "## 목적 / Purpose" are tokenised so either
//                       side matches a slot).
//   required_keywords — substring tokens matched case-insensitive
//                       against the whole body (e.g. "acceptance").
//
// Unset hints = fall back to the hard-coded defaults in
// artifact_propose.go (backward compat).
type validatorHints struct {
	RequiredH2       []string
	RequiredKeywords []string
}

var (
	validatorHintsMu    sync.RWMutex
	validatorHintsCache = map[string]*validatorHints{}
	// loaded tracks "we already tried to load this type, even if the
	// template was missing" so a nil return is cacheable. Without this
	// we'd re-query DB every preflight for types that never had a
	// template (e.g. Flow / TC / Feature).
	validatorHintsLoaded = map[string]bool{}
)

// validatorHintCommentRE extracts the payload between `<!-- validator:`
// and the closing `-->`. The payload is a `;`-separated list of
// `key=comma,values` pairs; extra whitespace around each part is OK.
var validatorHintCommentRE = regexp.MustCompile(`<!--\s*validator:\s*(.*?)\s*-->`)

// parseValidatorHints reads the validator meta comment out of a
// template body and returns the resolved hints. Returns nil when no
// comment is present so the caller can fall back to hard-coded defaults
// instead of inheriting an empty-list hint and silently disabling the
// gate.
func parseValidatorHints(body string) *validatorHints {
	m := validatorHintCommentRE.FindStringSubmatch(body)
	if m == nil {
		return nil
	}
	h := &validatorHints{}
	for _, segment := range strings.Split(m[1], ";") {
		kv := strings.SplitN(segment, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		raw := strings.TrimSpace(kv[1])
		if raw == "" {
			continue
		}
		var vals []string
		for _, v := range strings.Split(raw, ",") {
			v = strings.TrimSpace(v)
			if v != "" {
				vals = append(vals, v)
			}
		}
		switch key {
		case "required_h2":
			h.RequiredH2 = vals
		case "required_keywords":
			h.RequiredKeywords = vals
		}
	}
	return h
}

// getValidatorHints resolves hints for an artifact.type, caching both
// positive and negative lookups. Lookups are scoped to projectSlug because
// the _template_* seed lives per-project (migration 0006 seeds pindoc,
// pindoc.project.create seeds subsequent projects).
func getValidatorHints(ctx context.Context, deps Deps, projectSlug, artType string) *validatorHints {
	key := validatorHintsKey(projectSlug, artType)
	validatorHintsMu.RLock()
	hints := validatorHintsCache[key]
	loaded := validatorHintsLoaded[key]
	validatorHintsMu.RUnlock()
	if loaded {
		return hints
	}
	hints = loadTemplateHints(ctx, deps, projectSlug, artType)
	validatorHintsMu.Lock()
	validatorHintsCache[key] = hints
	validatorHintsLoaded[key] = true
	validatorHintsMu.Unlock()
	return hints
}

// invalidateValidatorHints clears the cache entry associated with a
// template slug. Called from handleUpdate's success path whenever a
// `_template_*` artifact is revised — the next propose for that type
// re-reads the updated meta comment.
func invalidateValidatorHints(projectSlug, templateSlug string) {
	if !strings.HasPrefix(templateSlug, "_template_") {
		return
	}
	typeKey := strings.TrimPrefix(templateSlug, "_template_")
	// DB slugs are lowercase, but artifact types are title-case
	// ("Task" / "Decision"). Keep cache keyed on lowercase type so the
	// invalidation matches getValidatorHints' normalisation.
	key := validatorHintsKey(projectSlug, typeKey)
	validatorHintsMu.Lock()
	delete(validatorHintsCache, key)
	delete(validatorHintsLoaded, key)
	validatorHintsMu.Unlock()
}

func validatorHintsKey(projectSlug, artType string) string {
	return projectSlug + "::" + strings.ToLower(strings.TrimSpace(artType))
}

// loadTemplateHints queries the _template_<lowercase-type> artifact in
// the current project and returns parsed hints. Returns nil when the
// row is missing, the DB is unreachable, or the body carries no
// validator comment — callers fall back to hard-coded defaults.
func loadTemplateHints(ctx context.Context, deps Deps, projectSlug, artType string) *validatorHints {
	if deps.DB == nil {
		return nil
	}
	slug := "_template_" + strings.ToLower(strings.TrimSpace(artType))
	var body string
	err := deps.DB.QueryRow(ctx, `
		SELECT a.body_markdown
		  FROM artifacts a
		  JOIN projects p ON p.id = a.project_id
		 WHERE p.slug = $1 AND a.slug = $2
		 LIMIT 1
	`, projectSlug, slug).Scan(&body)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		if deps.Logger != nil {
			deps.Logger.Warn("template hints lookup failed",
				"type", artType, "slug", slug, "err", err)
		}
		return nil
	}
	return parseValidatorHints(body)
}
