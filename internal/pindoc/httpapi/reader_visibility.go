package httpapi

import (
	"net/http"

	pauth "github.com/var-gg/pindoc/internal/pindoc/auth"
)

func readerHiddenProjectQueryRequested(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	q := r.URL.Query()
	return q.Get("include_hidden") == "true" ||
		q.Get("include_internal") == "true" ||
		q.Get("ops") == "1" ||
		q.Get("debug") == "ops"
}

func includeReaderHiddenProjects(r *http.Request, scope *pauth.ProjectScope) bool {
	return includeReaderHiddenProjectsForScope(readerHiddenProjectQueryRequested(r), scope)
}

func includeReaderHiddenProjectsForScope(requested bool, scope *pauth.ProjectScope) bool {
	return requested && scopeCanIncludeReaderHiddenProjects(scope)
}

func scopeCanIncludeReaderHiddenProjects(scope *pauth.ProjectScope) bool {
	return scope != nil && scope.Role == pauth.RoleOwner
}
