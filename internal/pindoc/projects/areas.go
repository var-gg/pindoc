// Package projects owns the project-bootstrap business logic — projects row
// + area taxonomy seed + template artifact seed — extracted from the
// pindoc.project.create MCP tool so HTTP, CLI, and UI entrypoints share
// the same source of truth (Decision
// project-bootstrap-canonical-flow-reader-ui-first-class).
package projects

import "strings"

// AreaSeed describes one project-root or sub-area row seeded at project
// create time. A nil ParentSlug on a TopLevelAreaSeedRow makes it a
// project-root (depth 0). StarterSubAreaSeed always carries a ParentSlug.
type AreaSeed struct {
	ParentSlug     string
	Slug           string
	Name           string
	Description    string
	IsCrossCutting bool
}

// TopLevelAreaSeedRow holds the bilingual descriptions used for the 9
// fixed root areas. Locale at create time picks one description; en
// stays as the fallback when ko isn't available or the project's
// primary_language is something we don't translate (yet).
type TopLevelAreaSeedRow struct {
	Slug           string
	Name           string
	DescriptionEN  string
	DescriptionKO  string
	IsCrossCutting bool
}

// TopLevelAreaSeed is the fixed 9-area skeleton every new project gets:
// 8 concern domains (strategy / context / experience / system / operations
// / governance / cross-cutting / misc) plus _unsorted as the
// reclassification queue. Decision
// area-구조-top-level-고정-골격-depth-2-sub-area만-프로젝트별-자유 froze the
// list and order; treat the slice as append-only — renaming a row breaks
// every dogfood project's URL.
var TopLevelAreaSeed = []TopLevelAreaSeedRow{
	{
		Slug:          "strategy",
		Name:          "Strategy",
		DescriptionEN: "Why this exists: vision, goals, scope, hypotheses, roadmap.",
		DescriptionKO: "프로젝트가 왜 존재하는지: 비전, 목표, 범위, 가설, 로드맵.",
	},
	{
		Slug:          "context",
		Name:          "Context",
		DescriptionEN: "External facts: users, competitors, literature, standards, external APIs.",
		DescriptionKO: "외부 사실: 사용자, 경쟁자, 문헌, 표준, 외부 API.",
	},
	{
		Slug:          "experience",
		Name:          "Experience",
		DescriptionEN: "What external actors see and do: UI, flows, IA, content, developer experience.",
		DescriptionKO: "외부 actor가 보고 겪는 것: UI, flow, IA, content, developer experience.",
	},
	{
		Slug:          "system",
		Name:          "System",
		DescriptionEN: "How it works internally: architecture, data, API, integrations, mechanisms, MCP, embedding.",
		DescriptionKO: "내부에서 작동하는 방식: architecture, data, API, integrations, mechanisms, MCP, embedding.",
	},
	{
		Slug:          "operations",
		Name:          "Operations",
		DescriptionEN: "How it ships, runs, and is supported: delivery, release, launch, incidents, editorial ops.",
		DescriptionKO: "출시·운영·지원 방식: delivery, release, launch, incidents, editorial ops.",
	},
	{
		Slug:          "governance",
		Name:          "Governance",
		DescriptionEN: "Rules, ownership, compliance, review, and taxonomy policy.",
		DescriptionKO: "규칙, ownership, compliance, review, taxonomy policy.",
	},
	{
		Slug:           "cross-cutting",
		Name:           "Cross-cutting",
		DescriptionEN:  "Reusable named concerns spanning multiple areas: security, privacy, accessibility, reliability, observability, localization.",
		DescriptionKO:  "여러 area에 반복 적용되는 named concern: security, privacy, accessibility, reliability, observability, localization.",
		IsCrossCutting: true,
	},
	{
		Slug:          "misc",
		Name:          "Misc",
		DescriptionEN: "Temporary overflow when no better subject area is clear.",
		DescriptionKO: "더 적절한 subject area가 불명확할 때 쓰는 임시 overflow.",
	},
	{
		Slug:          "_unsorted",
		Name:          "_Unsorted",
		DescriptionEN: "Quarantine queue for artifacts that need reclassification.",
		DescriptionKO: "재분류가 필요한 artifact를 잠시 두는 quarantine queue.",
	},
}

// StarterSubAreaSeeds is the depth-1 set every new project gets. Tuned
// for the V1 dogfood corpus — projects typically extend with their own
// sub-areas via pindoc.area.create as the artifact graph grows.
var StarterSubAreaSeeds = []AreaSeed{
	{"context", "users", "Users", "User research, personas, jobs, and needs.", false},
	{"context", "competitors", "Competitors", "Competitive analysis and adjacent products.", false},
	{"context", "literature", "Literature", "Literature review and external research.", false},
	{"context", "external-apis", "External APIs", "Third-party API facts, limits, contracts, and behavior.", false},
	{"context", "standards", "Standards", "External standards and protocol references.", false},
	{"context", "glossary", "Glossary", "Domain vocabulary and terminology context.", false},

	{"experience", "flows", "Flows", "User, agent, and developer-facing flows.", false},
	{"experience", "information-architecture", "Information architecture", "Navigation, hierarchy, and wayfinding.", false},
	{"experience", "content", "Content", "Reader copy, documentation content, and message structure.", false},
	{"experience", "developer-experience", "Developer experience", "Developer-facing setup, guidance, and ergonomics.", false},
	{"experience", "campaigns", "Campaigns", "Marketing or launch campaign experience.", false},

	{"system", "architecture", "Architecture", "System architecture and internal boundaries.", false},
	{"system", "data", "Data", "Schema, data model, migrations, and data contracts.", false},
	{"system", "mechanisms", "Mechanisms", "Internal mechanisms and runtime behavior.", false},
	{"system", "mcp", "MCP", "MCP tool contract and runtime surface.", false},
	{"system", "embedding", "Embedding", "Vector provider, chunking, dimensions, and retrieval substrate.", false},
	{"system", "api", "API", "Internal and external API contracts.", false},
	{"system", "integrations", "Integrations", "Integration boundaries and adapters.", false},

	{"operations", "delivery", "Delivery", "Delivery flow and handoff.", false},
	{"operations", "release", "Release", "Release process and notes.", false},
	{"operations", "launch", "Launch", "Launch operations and readiness.", false},
	{"operations", "incidents", "Incidents", "Incident response and postmortems.", false},
	{"operations", "editorial-ops", "Editorial ops", "Documentation and content operations.", false},
	{"operations", "community-ops", "Community ops", "Community support and moderation operations.", false},

	{"governance", "policies", "Policies", "Product and project policies.", false},
	{"governance", "compliance", "Compliance", "Compliance requirements and constraints.", false},
	{"governance", "ownership", "Ownership", "Ownership, accountability, and review boundaries.", false},
	{"governance", "review", "Review", "Review rules and approval gates.", false},
	{"governance", "taxonomy-policy", "Taxonomy policy", "Area taxonomy and classification governance.", false},

	{"cross-cutting", "security", "Security", "Security concern spanning multiple areas.", true},
	{"cross-cutting", "privacy", "Privacy", "Privacy concern spanning multiple areas.", true},
	{"cross-cutting", "accessibility", "Accessibility", "Accessibility concern spanning multiple areas.", true},
	{"cross-cutting", "reliability", "Reliability", "Reliability concern spanning multiple areas.", true},
	{"cross-cutting", "observability", "Observability", "Observability concern spanning multiple areas.", true},
	{"cross-cutting", "localization", "Localization", "Localization concern spanning multiple areas.", true},
}

// LocalizedAreaDescription picks the ko description when the project's
// primary language is ko (and a Korean string is provided), otherwise
// falls back to en. ja or other future languages reuse the en string
// until per-language seeds land.
func LocalizedAreaDescription(en, ko, lang string) string {
	if strings.EqualFold(strings.TrimSpace(lang), "ko") && strings.TrimSpace(ko) != "" {
		return ko
	}
	return en
}
