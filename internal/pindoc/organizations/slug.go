// Package organizations is the SaaS-ready multi-tenant primitive that
// lives in OSS too. An Organization is the billing-and-ownership boundary
// shared by Slack/Notion/Linear/GitHub: a user can belong to many Orgs,
// every Project hangs off exactly one Org, and per-seat plans attach to
// an Org (not a User) once the SaaS layer lands.
//
// The schema hangs projects from projects.organization_id (UUID FK).
// The old owner-label transition column was removed after the API
// surfaces moved to the Organization model.
package organizations

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// orgSlugRe enforces the same kebab-case shape Project slugs use, so the
// /pindoc.org/{slug} route can resolve to either an Organization or a
// Project sub-path without ambiguous parsing. Leading letter, 2-40 chars.
var orgSlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)

// reservedOrgSlugs blocks slugs that would collide with the top-level URL
// surface (login, api, docs, settings, ...) or with future system paths
// the hosted product needs to claim. Kept slightly broader than the
// Project reserved list because Org slugs sit at a higher routing level
// (/{slug}/...) where any first-segment collision breaks the whole tree.
//
// Shared namespace: users.username (the personal-Org slug) lives in the
// same TEXT column space as organizations.slug. Anything reserved here
// is also off-limits as a username, so /pindoc.org/{handle} routing is
// unambiguous.
var reservedOrgSlugs = map[string]struct{}{
	// Auth + onboarding
	"login": {}, "logout": {}, "signup": {}, "signin": {}, "register": {},
	"oauth": {}, "auth": {}, "sso": {}, "verify": {}, "confirm": {},
	"forgot": {}, "reset": {}, "invite": {},
	// System / admin
	"admin": {}, "api": {}, "app": {}, "www": {}, "internal": {},
	"settings": {}, "dashboard": {}, "console": {}, "system": {},
	"health": {}, "status": {}, "metrics": {}, "ping": {},
	// Marketing / public site
	"about": {}, "blog": {}, "docs": {}, "help": {}, "support": {},
	"contact": {}, "home": {}, "index": {}, "pricing": {}, "plans": {},
	"terms": {}, "privacy": {}, "security": {}, "legal": {}, "cookies": {},
	"changelog": {}, "roadmap": {}, "press": {}, "careers": {},
	"showcase": {}, "gallery": {}, "explore": {}, "discover": {},
	"features": {}, "product": {}, "company": {}, "team": {},
	// URL/routing reservations
	"p": {}, "u": {}, "user": {}, "users": {}, "org": {}, "orgs": {},
	"organization": {}, "organizations": {}, "workspace": {}, "workspaces": {},
	"new": {}, "create": {}, "edit": {}, "delete": {},
	"static": {}, "assets": {}, "public": {}, "cdn": {}, "_next": {},
	"images": {}, "img": {}, "media": {}, "files": {}, "uploads": {},
	// Common service paths
	"mail": {}, "email": {}, "smtp": {}, "ftp": {}, "ssh": {},
	"webhook": {}, "webhooks": {}, "callback": {}, "callbacks": {},
	"billing": {}, "payment": {}, "payments": {}, "invoice": {}, "invoices": {},
	"checkout": {}, "subscribe": {}, "subscription": {},
	// Reserved sentinel
	"default": {}, // application-bootstrap default Org uses this slug
}

// Sentinel errors so callers can branch on stable codes without parsing
// error strings. Mirrors the projects package convention.
var (
	ErrSlugInvalid  = errors.New("ORG_SLUG_INVALID")
	ErrSlugReserved = errors.New("ORG_SLUG_RESERVED")
	ErrSlugTaken    = errors.New("ORG_SLUG_TAKEN")
	ErrNameRequired = errors.New("ORG_NAME_REQUIRED")
	ErrKindInvalid  = errors.New("ORG_KIND_INVALID")
	ErrNotFound     = errors.New("ORG_NOT_FOUND")
)

// IsReservedSlug exposes the reserved set so other packages (users.username
// validation, the /api/orgs/check-availability handler) can share the same
// blocklist without re-implementing the rule. The 'default' sentinel is
// included — application code may use it internally but a real signup
// must not pick it.
func IsReservedSlug(slug string) bool {
	_, reserved := reservedOrgSlugs[strings.ToLower(strings.TrimSpace(slug))]
	return reserved
}

// ValidateSlug runs the static checks (regex + reserved list) so callers
// can give live feedback on user input before paying for a tx. CreateOrg
// re-runs the same check defensively, so a caller that skips this still
// gets the same outcome — just one round-trip later.
func ValidateSlug(slug string) error {
	s := strings.ToLower(strings.TrimSpace(slug))
	if !orgSlugRe.MatchString(s) {
		return fmt.Errorf("%w: slug must be lowercase kebab-case (2-40 chars, starts with a letter): got %q", ErrSlugInvalid, slug)
	}
	if _, reserved := reservedOrgSlugs[s]; reserved {
		return fmt.Errorf("%w: slug %q collides with reserved system paths (/login, /api, /settings, /docs, ...). Pick something specific to your team or username", ErrSlugReserved, s)
	}
	return nil
}

// NormalizeSlug returns the canonical lower-case trimmed slug. Returns
// the same value the validator and DB layer expect — call it before any
// uniqueness check or insert so case-folded lookups stay consistent.
func NormalizeSlug(slug string) string {
	return strings.ToLower(strings.TrimSpace(slug))
}
