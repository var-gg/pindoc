// Package projects owns the project-bootstrap business logic — projects row
// + area taxonomy seed + template artifact seed — extracted from the
// pindoc.project.create MCP tool so HTTP, CLI, and UI entrypoints share
// the same source of truth (Decision
// project-bootstrap-canonical-flow-reader-ui-first-class).
package projects

import (
	"fmt"
	"strings"
)

// AreaSeed describes one sub-area row seeded at project create time.
// Every StarterSubAreas entry carries a ParentSlug naming its top-level.
type AreaSeed struct {
	ParentSlug     string
	Slug           string
	Name           string
	Description    string
	IsCrossCutting bool
}

// TopLevelAreaSeedRow holds the bilingual descriptions for one root area
// of a taxonomy profile. Locale at create time picks one description; en
// stays as the fallback when ko isn't available or the project's
// primary_language is something we don't translate yet.
type TopLevelAreaSeedRow struct {
	Slug           string
	Name           string
	DescriptionEN  string
	DescriptionKO  string
	IsCrossCutting bool
}

// TaxonomyProfile is the per-domain area skeleton a project picks at
// create time. Decision area-taxonomy-profiled-skeleton amended the
// universal 8-concern skeleton into profile-governed top-level areas:
// each project records a profile slug + version, and project.create
// seeds that profile's top-level + starter sub-areas.
//
// Invariant: every profile MUST include the `misc` (overflow) and
// `_unsorted` (quarantine) top-level areas. seedTemplates resolves the
// `misc` area by slug, and area_taxonomy_metrics counts `misc` by slug —
// a profile without them breaks project.create and the metrics function.
// ValidateTaxonomyRegistry enforces this; create_test.go runs it.
type TaxonomyProfile struct {
	Slug            string
	Version         string
	TopLevel        []TopLevelAreaSeedRow
	StarterSubAreas []AreaSeed
}

// DefaultTaxonomyProfileSlug is assigned to a project that does not pick
// a profile explicitly, and is backfilled onto every pre-existing
// project by migration 0063_taxonomy_profiles.
const DefaultTaxonomyProfileSlug = "software-product"

// TaxonomyProfiles is the profile registry keyed by slug. project.create
// looks up the chosen profile here; an unknown or empty slug falls back
// to DefaultTaxonomyProfileSlug via TaxonomyProfileBySlug.
var TaxonomyProfiles = map[string]TaxonomyProfile{
	softwareProductProfile.Slug: softwareProductProfile,
	gameNarrativeProfile.Slug:   gameNarrativeProfile,
}

// TaxonomyProfileBySlug returns the registered profile for slug, or the
// default profile when slug is empty or unknown. The bool reports
// whether slug matched a registered profile exactly.
func TaxonomyProfileBySlug(slug string) (TaxonomyProfile, bool) {
	if p, ok := TaxonomyProfiles[strings.TrimSpace(slug)]; ok {
		return p, true
	}
	return TaxonomyProfiles[DefaultTaxonomyProfileSlug], false
}

// ValidateTaxonomyRegistry enforces the per-profile invariants every
// registered profile must hold: a matching registry key, the required
// `misc`/`_unsorted` top-level areas, no duplicate slugs, and a known
// parent for every sub-area. create_test.go calls this so a malformed
// profile fails `go test` rather than a project.create at runtime.
func ValidateTaxonomyRegistry() error {
	for key, p := range TaxonomyProfiles {
		if p.Slug != key {
			return fmt.Errorf("profile registered under key %q but Slug is %q", key, p.Slug)
		}
		if strings.TrimSpace(p.Version) == "" {
			return fmt.Errorf("profile %q has empty Version", key)
		}
		tops := map[string]bool{}
		for _, t := range p.TopLevel {
			if tops[t.Slug] {
				return fmt.Errorf("profile %q has duplicate top-level %q", key, t.Slug)
			}
			tops[t.Slug] = true
		}
		for _, required := range []string{"misc", "_unsorted"} {
			if !tops[required] {
				return fmt.Errorf("profile %q is missing required top-level %q", key, required)
			}
		}
		subs := map[string]bool{}
		for _, s := range p.StarterSubAreas {
			if subs[s.Slug] {
				return fmt.Errorf("profile %q has duplicate sub-area %q", key, s.Slug)
			}
			subs[s.Slug] = true
			if !tops[s.ParentSlug] {
				return fmt.Errorf("profile %q sub-area %q names unknown parent %q", key, s.Slug, s.ParentSlug)
			}
		}
	}
	return nil
}

// softwareProductProfile is the original 8-concern skeleton + _unsorted,
// frozen by Decision area-taxonomy-reform-path-a and retained verbatim
// so existing projects backfilled to this profile keep identical URLs.
// Treat TopLevel as append-only: renaming a row breaks every artifact
// URL under that area.
var softwareProductProfile = TaxonomyProfile{
	Slug:    "software-product",
	Version: "reform-v1",
	TopLevel: []TopLevelAreaSeedRow{
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
	},
	StarterSubAreas: []AreaSeed{
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
	},
}

// gameNarrativeProfile is the first non-software profile, drafted in
// Analysis game-narrative-profile-draft. TopLevel raises combat /
// characters / narrative / atlas / art to first-class areas; `project`
// folds software-style strategy/operations/governance into one
// structural shelf. StarterSubAreas here is the minimum seed set — the
// broader lazy catalog is materialized on first use by Task
// task-profile-aware-area-seed.
var gameNarrativeProfile = TaxonomyProfile{
	Slug:    "game-narrative",
	Version: "draft-v1",
	TopLevel: []TopLevelAreaSeedRow{
		{
			Slug:          "project",
			Name:          "Project",
			DescriptionEN: "Project-level meta concern: direction, research, roadmap, production, governance.",
			DescriptionKO: "게임이 아니라 프로젝트 자체의 meta concern: 방향, 리서치, 로드맵, 제작, 거버넌스.",
		},
		{
			Slug:          "gameplay",
			Name:          "Gameplay",
			DescriptionEN: "Core play rules outside combat: core loop, progression, economy, survival.",
			DescriptionKO: "전투 외 플레이 규칙: core loop, progression, economy, survival.",
		},
		{
			Slug:          "combat",
			Name:          "Combat",
			DescriptionEN: "Combat rules, encounter design, enemy behavior, and balance.",
			DescriptionKO: "전투 규칙, encounter 설계, enemy behavior, balance.",
		},
		{
			Slug:          "characters",
			Name:          "Characters",
			DescriptionEN: "Character lore, cast structure, factions, and relationships.",
			DescriptionKO: "캐릭터 lore, cast 구조, faction·relationship 모델.",
		},
		{
			Slug:          "narrative",
			Name:          "Narrative",
			DescriptionEN: "Plot, quests, dialogue, branching, canon, and themes.",
			DescriptionKO: "plot, quest, dialogue, branching, canon, theme.",
		},
		{
			Slug:          "atlas",
			Name:          "Atlas",
			DescriptionEN: "World map, regions, locations, biomes, and traversal.",
			DescriptionKO: "지도, 지역, 장소, 생태권, traversal.",
		},
		{
			Slug:          "art",
			Name:          "Art",
			DescriptionEN: "Visual direction, concept and production art, UI and audio.",
			DescriptionKO: "visual direction, concept·production art, UI·audio.",
		},
		{
			Slug:          "implementation",
			Name:          "Implementation",
			DescriptionEN: "Game implementation: engine, tools, data, content pipeline, builds.",
			DescriptionKO: "게임 구현: engine, tools, data, content pipeline, build.",
		},
		{
			Slug:           "cross-cutting",
			Name:           "Cross-cutting",
			DescriptionEN:  "Reusable named concerns spanning domains: accessibility, localization, performance, content safety.",
			DescriptionKO:  "여러 도메인에 반복 적용되는 named concern: accessibility, localization, performance, content safety.",
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
	},
	StarterSubAreas: []AreaSeed{
		{"project", "direction", "Direction", "Product and project direction and goals.", false},
		{"project", "research", "Research", "External research and reference gathering.", false},
		{"project", "roadmap", "Roadmap", "Release plan and milestones.", false},
		{"project", "production", "Production", "Production process and pipeline coordination.", false},
		{"project", "governance", "Governance", "Project rules, ownership, and review.", false},

		{"gameplay", "core-loop", "Core loop", "The central moment-to-moment gameplay loop.", false},
		{"gameplay", "progression", "Progression", "Player progression, leveling, and unlocks.", false},
		{"gameplay", "economy", "Economy", "In-game economy, currency, and resource balance.", false},
		{"gameplay", "survival", "Survival", "Survival, crafting, and resource-management systems.", false},

		{"combat", "rules", "Rules", "Combat rules and core mechanics.", false},
		{"combat", "encounters", "Encounters", "Encounter design and pacing.", false},
		{"combat", "enemies", "Enemies", "Enemy types and behavior.", false},
		{"combat", "balance", "Balance", "Weapon, ability, and difficulty balance.", false},

		{"characters", "heroes", "Heroes", "Player-side hero characters.", false},
		{"characters", "npcs", "NPCs", "Non-player characters.", false},
		{"characters", "antagonists", "Antagonists", "Antagonist and villain characters.", false},
		{"characters", "factions", "Factions", "Factions and their relationships.", false},
		{"characters", "relationships", "Relationships", "Cast-wide relationship and affinity model.", false},

		{"narrative", "plot", "Plot", "Main plot and story arcs.", false},
		{"narrative", "quests", "Quests", "Quest design and structure.", false},
		{"narrative", "dialogue", "Dialogue", "Dialogue writing and branching.", false},
		{"narrative", "scenes", "Scenes", "Cutscenes and scripted scenes.", false},
		{"narrative", "themes", "Themes", "Narrative themes and tone.", false},

		{"atlas", "world-map", "World map", "Overall world map and geography.", false},
		{"atlas", "regions", "Regions", "Region definitions and boundaries.", false},
		{"atlas", "locations", "Locations", "Individual locations and points of interest.", false},
		{"atlas", "biomes", "Biomes", "Biomes and environmental ecology.", false},
		{"atlas", "routes", "Routes", "Traversal routes and connectivity.", false},

		{"art", "visual-style", "Visual style", "Overall visual direction and style guide.", false},
		{"art", "concept-art", "Concept art", "Concept art and visual exploration.", false},
		{"art", "character-art", "Character art", "Character art and design.", false},
		{"art", "environment-art", "Environment art", "Environment and prop art.", false},
		{"art", "ui-audio", "UI & audio", "UI presentation and audio direction.", false},

		{"implementation", "engine", "Engine", "Game engine and runtime systems.", false},
		{"implementation", "tools", "Tools", "Editor and pipeline tooling.", false},
		{"implementation", "data", "Data", "Data schema and content data contracts.", false},
		{"implementation", "content-pipeline", "Content pipeline", "Asset and content build pipeline.", false},
		{"implementation", "builds", "Builds", "Build, packaging, and release mechanics.", false},

		{"cross-cutting", "accessibility", "Accessibility", "Accessibility concern spanning multiple domains.", true},
		{"cross-cutting", "localization", "Localization", "Localization concern spanning multiple domains.", true},
		{"cross-cutting", "performance", "Performance", "Performance concern spanning multiple domains.", true},
		{"cross-cutting", "content-safety", "Content safety", "Content safety and rating concern spanning multiple domains.", true},
	},
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
