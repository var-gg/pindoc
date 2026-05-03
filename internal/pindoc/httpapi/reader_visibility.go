package httpapi

import (
	"net/http"
	"strings"
)

var readerHiddenProjectPrefixes = []string{
	"oauth-it-",
	"invite-http-",
	"workspace-detect-",
	"vis-http-",
	"vis-mcp-",
	"artifact-audit-",
	"task-flow-a-",
	"task-flow-b-",
	"task-queue-across-a-",
	"task-queue-across-b-",
}

func readerHiddenProjectSlug(slug string) bool {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if readerHiddenPindocHexFixtureSlug(slug) {
		return true
	}
	for _, prefix := range readerHiddenProjectPrefixes {
		if strings.HasPrefix(slug, prefix) {
			return true
		}
	}
	return false
}

func readerHiddenPindocHexFixtureSlug(slug string) bool {
	const prefix = "pindoc-"
	if !strings.HasPrefix(slug, prefix) {
		return false
	}
	hex := strings.TrimPrefix(slug, prefix)
	if len(hex) != 16 {
		return false
	}
	for _, c := range hex {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func includeReaderHiddenProjects(r *http.Request) bool {
	q := r.URL.Query()
	return q.Get("include_hidden") == "true" ||
		q.Get("include_internal") == "true" ||
		q.Get("ops") == "1" ||
		q.Get("debug") == "ops"
}
