package httpapi

import (
	"net/http"
	"strings"
)

var readerHiddenProjectPrefixes = []string{
	"oauth-it-",
	"invite-http-",
	"workspace-detect-",
}

func readerHiddenProjectSlug(slug string) bool {
	slug = strings.ToLower(strings.TrimSpace(slug))
	for _, prefix := range readerHiddenProjectPrefixes {
		if strings.HasPrefix(slug, prefix) {
			return true
		}
	}
	return false
}

func includeReaderHiddenProjects(r *http.Request) bool {
	q := r.URL.Query()
	return q.Get("include_hidden") == "true" ||
		q.Get("include_internal") == "true" ||
		q.Get("ops") == "1" ||
		q.Get("debug") == "ops"
}
