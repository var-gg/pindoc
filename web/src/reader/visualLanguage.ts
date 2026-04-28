export type VisualLocale = "en" | "ko";

export type VisualEntry = {
  label_en: string;
  label_ko: string;
  description_en: string;
  description_ko: string;
  icon: string;
  color_token: string;
  fixed?: boolean;
  parent?: string | null;
  signature_color?: boolean;
};

export type VisualTypeEntry = VisualEntry & {
  canonical: string;
  variant: string;
};

export type VisualMetaEnumKey =
  | "source_type"
  | "consent_state"
  | "confidence"
  | "audience"
  | "next_context_policy"
  | "verification_state";

export const visualLanguage = {
  types: {
    decision: typeEntry("Decision", "decision", "FileCheck2", "--type-decision", "Decision", "결정", "Choice, rationale, alternatives, and consequences.", "선택, 근거, 대안, 결과를 남기는 문서."),
    analysis: typeEntry("Analysis", "analysis", "FileSearch", "--type-analysis", "Analysis", "분석", "Evidence, tradeoffs, or investigation without a final decision.", "최종 결정 전 근거, 비교, 조사를 정리하는 문서."),
    task: typeEntry("Task", "task", "CheckSquare", "--type-task", "Task", "작업", "Work item with owner, status, and acceptance checks.", "담당, 상태, acceptance check를 가진 작업 항목."),
    debug: typeEntry("Debug", "debug", "Bug", "--type-debug", "Debug", "디버그", "Failure report, reproduction, hypothesis, and fix notes.", "장애 재현, 가설, 수정 기록을 담는 문서."),
    glossary: typeEntry("Glossary", "glossary", "BookOpen", "--type-glossary", "Glossary", "용어집", "Term definition and boundary for shared language.", "공유 언어를 위한 용어 정의와 경계."),
    flow: typeEntry("Flow", "flow", "Workflow", "--type-flow", "Flow", "흐름", "User, system, or agent sequence across steps.", "사용자, 시스템, agent의 단계 흐름."),
    tc: typeEntry("TC", "tc", "BadgeCheck", "--type-tc", "Test case", "테스트 케이스", "Test case or checkable scenario.", "확인 가능한 test case 또는 시나리오."),
    feature: typeEntry("Feature", "feature", "Sparkles", "--type-feature", "Feature", "기능", "Product capability or behavior users can experience.", "사용자가 경험하는 제품 기능이나 행동."),
    apiendpoint: typeEntry("APIEndpoint", "apiendpoint", "Code2", "--type-apiendpoint", "API endpoint", "API 엔드포인트", "HTTP/API contract, request shape, and response shape.", "HTTP/API 계약, 요청 형태, 응답 형태."),
    screen: typeEntry("Screen", "screen", "Monitor", "--type-screen", "Screen", "화면", "User-facing UI surface and state model.", "사용자에게 보이는 UI surface와 상태 모델."),
    datamodel: typeEntry("DataModel", "datamodel", "Database", "--type-datamodel", "Data model", "데이터 모델", "Entity, field, relation, and storage structure.", "엔티티, 필드, 관계, 저장 구조."),
  },
  areas: {
    strategy: areaEntry("Target", "--area-strategy", "Strategy", "전략", "Direction, goals, scope, roadmap, and product bets.", "방향, 목표, scope, roadmap, 제품 가설.", true, null),
    context: areaEntry("Globe2", "--area-context", "Context", "맥락", "External facts, research, competitors, and standards.", "외부 사실, 리서치, 경쟁사, 표준.", true, null),
    experience: areaEntry("PanelTop", "--area-experience", "Experience", "사용자 경험", "User or developer-visible screens, flows, and content.", "사용자나 개발자가 보는 화면, flow, content.", true, null),
    system: areaEntry("Cpu", "--area-system", "System", "시스템", "Internal architecture, data, APIs, runtime, and mechanisms.", "내부 architecture, data, API, runtime, mechanism.", true, null),
    operations: areaEntry("Wrench", "--area-operations", "Operations", "운영", "Delivery, release, deployment, incident, and support work.", "delivery, release, deployment, incident, support.", true, null),
    governance: areaEntry("Scale", "--area-governance", "Governance", "거버넌스", "Policy, permission, ownership, review, and taxonomy rules.", "policy, permission, ownership, review, taxonomy rule.", true, null),
    "cross-cutting": areaEntry("Layers3", "--area-cross-cutting", "Cross-cutting", "횡단 관심사", "Reusable concerns that apply across multiple areas.", "여러 area에 반복 적용되는 재사용 concern.", true, null),
    misc: areaEntry("Archive", "--area-misc", "Misc", "기타", "Temporary overflow when the primary shelf is unclear.", "primary shelf가 아직 불명확할 때 쓰는 임시 overflow.", true, null),
    security: areaEntry("ShieldCheck", "--area-security", "Security", "보안", "Security controls and abuse-resistance across the system.", "시스템 전반의 보안 통제와 abuse-resistance.", false, "cross-cutting"),
    privacy: areaEntry("Lock", "--area-privacy", "Privacy", "개인정보", "Privacy boundaries, retention, consent, and sensitive data handling.", "개인정보 경계, 보존, 동의, 민감 데이터 처리.", false, "cross-cutting"),
    accessibility: areaEntry("Accessibility", "--area-accessibility", "Accessibility", "접근성", "Inclusive interaction, keyboard access, contrast, and assistive technology support.", "포용적 상호작용, 키보드 접근, 대비, 보조기술 지원.", false, "cross-cutting"),
    reliability: areaEntry("Gauge", "--area-reliability", "Reliability", "신뢰성", "Availability, failure behavior, recovery, and operational resilience.", "가용성, 장애 동작, 복구, 운영 탄력성.", false, "cross-cutting"),
    observability: areaEntry("Activity", "--area-observability", "Observability", "관측성", "Telemetry, logs, metrics, traces, and operator visibility.", "텔레메트리, 로그, 메트릭, trace, 운영자 가시성.", false, "cross-cutting"),
    localization: areaEntry("Languages", "--area-localization", "Localization", "현지화", "Language, locale, translation, and culturally appropriate presentation.", "언어, locale, 번역, 문화권에 맞는 표현.", false, "cross-cutting"),
  },
  relations: {
    implements: relationEntry("Puzzle", "--relation-implements", "implements", "구현", "This artifact implements the target decision or task.", "이 artifact가 대상 decision 또는 task를 구현함."),
    references: relationEntry("Link2", "--relation-references", "references", "참조", "This artifact cites the target as supporting context.", "이 artifact가 대상을 근거 맥락으로 참조함."),
    blocks: relationEntry("Ban", "--relation-blocks", "blocks", "블록", "This artifact blocks progress on the target.", "이 artifact가 대상 진행을 막고 있음."),
    relates_to: relationEntry("ArrowLeftRight", "--relation-relates-to", "relates to", "관련", "Loose relationship without a stronger semantic edge.", "더 강한 의미 관계는 아니지만 서로 관련됨."),
    translation_of: relationEntry("Languages", "--relation-translation-of", "translation of", "번역", "This artifact is a translation of the target.", "이 artifact가 대상의 번역본임."),
  },
  pins: {
    code: pinEntry("Code2", "--pin-code", "Code", "코드", "Repository file, commit, or line-range evidence.", "repository 파일, commit, line-range 근거."),
    doc: pinEntry("FileText", "--pin-doc", "Doc", "문서", "Markdown, text, README, changelog, or license evidence.", "Markdown, 텍스트, README, changelog, license 근거."),
    config: pinEntry("Settings2", "--pin-config", "Config", "설정", "Configuration file such as JSON, YAML, env, Dockerfile, or build config.", "JSON, YAML, env, Dockerfile, build config 같은 설정 파일."),
    asset: pinEntry("Image", "--pin-asset", "Asset", "에셋", "Image, PDF, media, font, or other binary/design asset.", "이미지, PDF, media, font 또는 binary/design 에셋."),
    resource: pinEntry("Package", "--pin-resource", "Resource", "리소스", "Typed internal resource reference.", "타입이 있는 내부 resource 참조."),
    url: pinEntry("ExternalLink", "--pin-url", "URL", "URL", "External web link or source URL.", "외부 web link 또는 출처 URL."),
  },
  meta_enums: {
    source_type: {
      code: metaEntry("Code2", "--meta-source", "Code", "코드", "Repository code is the main evidence.", "repository code가 주 근거."),
      artifact: metaEntry("FileText", "--meta-source", "Artifact-derived", "Artifact 기반", "Derived primarily from other Pindoc artifacts.", "다른 Pindoc artifact에서 주로 파생됨."),
      user_chat: metaEntry("MessageSquare", "--meta-source", "Conversation-derived", "대화 기반", "Derived from a user conversation turn.", "사용자 대화 turn에서 파생됨."),
      external: metaEntry("ExternalLink", "--meta-source", "External", "외부 자료", "Derived from external docs, specs, or URLs.", "외부 문서, 스펙, URL에서 파생됨."),
      mixed: metaEntry("Blend", "--meta-source", "Mixed", "혼합 근거", "Evidence combines code, artifacts, chat, or external sources.", "코드, artifact, 대화, 외부 자료가 섞인 근거."),
    },
    consent_state: {
      not_needed: metaEntry("CircleMinus", "--meta-consent", "Consent not needed", "동의 불필요", "No user-chat consent is needed for this content.", "이 내용에는 사용자 대화 동의가 필요하지 않음."),
      requested: metaEntry("CircleHelp", "--meta-consent", "Consent requested", "동의 요청됨", "Consent has been requested but not resolved.", "동의를 요청했지만 아직 확정되지 않음."),
      granted: metaEntry("CircleCheck", "--meta-consent", "Consent granted", "동의 승인", "The user granted reuse/canonicalization consent.", "사용자가 재사용 또는 canonicalization 동의를 승인함."),
      denied: metaEntry("CircleX", "--meta-consent", "Consent denied", "동의 거부", "The user denied reuse/canonicalization consent.", "사용자가 재사용 또는 canonicalization 동의를 거부함."),
    },
    confidence: {
      low: metaEntry("SignalLow", "--meta-confidence", "Confidence: low", "신뢰도 낮음", "The authoring agent reported low confidence.", "작성 agent가 낮은 confidence를 보고함."),
      medium: metaEntry("SignalMedium", "--meta-confidence", "Confidence: medium", "신뢰도 보통", "The authoring agent reported medium confidence.", "작성 agent가 보통 confidence를 보고함."),
      high: metaEntry("SignalHigh", "--meta-confidence", "Confidence: high", "신뢰도 높음", "The authoring agent reported high confidence.", "작성 agent가 높은 confidence를 보고함."),
    },
    audience: {
      owner_only: metaEntry("UserLock", "--meta-audience", "Owner only", "소유자 전용", "Only the owner should see this artifact.", "소유자에게만 공개해야 하는 artifact."),
      approvers: metaEntry("UsersRound", "--meta-audience", "Approvers only", "승인자 전용", "Limited to approvers or reviewers.", "승인자 또는 reviewer에게 제한됨."),
      project_readers: metaEntry("BookOpen", "--meta-audience", "Project readers", "프로젝트 독자", "Visible to normal project readers.", "일반 프로젝트 독자에게 노출 가능."),
    },
    next_context_policy: {
      default: metaEntry("ListPlus", "--meta-context", "Context: default", "컨텍스트: 기본", "Eligible for default next-session context.", "다음 세션 기본 context 후보."),
      opt_in: metaEntry("MousePointerClick", "--meta-context", "Context: opt-in", "컨텍스트: 요청 시", "Only surfaces when directly searched or requested.", "직접 검색 또는 요청될 때만 노출."),
      excluded: metaEntry("ListX", "--meta-context", "Excluded from context", "컨텍스트 제외", "Skipped from default next-session context.", "다음 세션 기본 context에서 제외."),
    },
    verification_state: {
      verified: metaEntry("BadgeCheck", "--meta-verification", "Verified", "검증됨", "Checked against code or authoritative evidence.", "코드나 권위 있는 근거로 확인됨."),
      partially_verified: metaEntry("Badge", "--meta-verification", "Partially verified", "부분 검증", "Some evidence exists, but the artifact is not fully verified.", "일부 근거는 있으나 전체 검증은 아님."),
      unverified: metaEntry("BadgeAlert", "--meta-verification", "Unverified", "미검증", "No verification evidence is attached.", "확인 근거가 붙어 있지 않음."),
    },
  },
  quick_actions: {
    verify_request: quickActionEntry("CheckCircle2", "--quick-verify", "Copy verification request", "검증 요청 복사", "Copy a request that asks an agent to verify this artifact.", "이 artifact 검증을 요청하는 문구를 복사."),
    update_request: quickActionEntry("Pencil", "--quick-update", "Copy update request", "수정 요청 복사", "Copy a request that asks an agent to update this artifact.", "이 artifact 수정을 요청하는 문구를 복사."),
    copy_link: quickActionEntry("Share2", "--quick-copy-link", "Copy share link", "공유 링크 복사", "Copy the browser URL for this artifact.", "이 artifact의 브라우저 URL을 복사."),
    copy_agent_ref: quickActionEntry("Clipboard", "--quick-agent-ref", "Copy agent ref", "agent ref 복사", "Copy the pindoc:// reference for agent prompts.", "agent prompt에 쓰는 pindoc:// 참조를 복사."),
    history: quickActionEntry("History", "--quick-history", "Open revision history", "수정 이력 열기", "Open the revision timeline for this artifact.", "이 artifact의 revision timeline을 열기."),
  },
} as const;

export const topLevelVisualAreaSlugs = [
  "strategy",
  "context",
  "experience",
  "system",
  "operations",
  "governance",
  "cross-cutting",
  "misc",
] as const;

export function visualLabel(entry: VisualEntry, locale: string): string {
  return locale === "ko" ? entry.label_ko : entry.label_en;
}

export function visualDescription(entry: VisualEntry, locale: string): string {
  return locale === "ko" ? entry.description_ko : entry.description_en;
}

export function normalizeTypeKey(type: string | undefined | null): keyof typeof visualLanguage.types | null {
  if (!type) return null;
  const variant = type.toLowerCase().replace(/[^a-z]/g, "");
  return variant in visualLanguage.types ? variant as keyof typeof visualLanguage.types : null;
}

export function visualType(type: string | undefined | null): VisualTypeEntry | null {
  const key = normalizeTypeKey(type);
  return key ? visualLanguage.types[key] : null;
}

export function visualTypeVariant(type: string | undefined | null): string | null {
  return visualType(type)?.variant ?? null;
}

export function visualArea(slug: string | undefined | null): VisualEntry | null {
  if (!slug) return null;
  return slug in visualLanguage.areas
    ? visualLanguage.areas[slug as keyof typeof visualLanguage.areas]
    : null;
}

export function visualRelation(relation: string | undefined | null): VisualEntry | null {
  if (!relation) return null;
  const key = relation.toLowerCase() as keyof typeof visualLanguage.relations;
  return key in visualLanguage.relations ? visualLanguage.relations[key] : null;
}

export function visualRelationClass(relation: string | undefined | null): string {
  if (!relation) return "default";
  const entry = visualRelation(relation);
  if (entry) return relation.toLowerCase().replace(/_/g, "-");
  return relation.toLowerCase().replace(/[^a-z0-9]+/g, "-") || "default";
}

export function visualPin(kind: string | undefined | null): VisualEntry | null {
  if (!kind) return null;
  const key = kind.toLowerCase() as keyof typeof visualLanguage.pins;
  return key in visualLanguage.pins ? visualLanguage.pins[key] : null;
}

export function visualMetaEnum(enumKey: VisualMetaEnumKey, value: string | undefined | null): VisualEntry | null {
  if (!value) return null;
  const group = visualLanguage.meta_enums[enumKey] as Record<string, VisualEntry>;
  return group[value] ?? null;
}

export function visualQuickAction(action: keyof typeof visualLanguage.quick_actions): VisualEntry {
  return visualLanguage.quick_actions[action];
}

function typeEntry(
  canonical: string,
  variant: string,
  icon: string,
  colorToken: string,
  labelEn: string,
  labelKo: string,
  descriptionEn: string,
  descriptionKo: string,
): VisualTypeEntry {
  return {
    canonical,
    variant,
    icon,
    color_token: colorToken,
    label_en: labelEn,
    label_ko: labelKo,
    description_en: descriptionEn,
    description_ko: descriptionKo,
    fixed: true,
  };
}

function areaEntry(
  icon: string,
  colorToken: string,
  labelEn: string,
  labelKo: string,
  descriptionEn: string,
  descriptionKo: string,
  signatureColor: boolean,
  parent: string | null,
): VisualEntry {
  return {
    icon,
    color_token: colorToken,
    label_en: labelEn,
    label_ko: labelKo,
    description_en: descriptionEn,
    description_ko: descriptionKo,
    fixed: true,
    parent,
    signature_color: signatureColor,
  };
}

function relationEntry(icon: string, colorToken: string, labelEn: string, labelKo: string, descriptionEn: string, descriptionKo: string): VisualEntry {
  return {
    icon,
    color_token: colorToken,
    label_en: labelEn,
    label_ko: labelKo,
    description_en: descriptionEn,
    description_ko: descriptionKo,
    fixed: true,
  };
}

function pinEntry(icon: string, colorToken: string, labelEn: string, labelKo: string, descriptionEn: string, descriptionKo: string): VisualEntry {
  return {
    icon,
    color_token: colorToken,
    label_en: labelEn,
    label_ko: labelKo,
    description_en: descriptionEn,
    description_ko: descriptionKo,
    fixed: true,
  };
}

function metaEntry(icon: string, colorToken: string, labelEn: string, labelKo: string, descriptionEn: string, descriptionKo: string): VisualEntry {
  return {
    icon,
    color_token: colorToken,
    label_en: labelEn,
    label_ko: labelKo,
    description_en: descriptionEn,
    description_ko: descriptionKo,
    fixed: true,
  };
}

function quickActionEntry(icon: string, colorToken: string, labelEn: string, labelKo: string, descriptionEn: string, descriptionKo: string): VisualEntry {
  return {
    icon,
    color_token: colorToken,
    label_en: labelEn,
    label_ko: labelKo,
    description_en: descriptionEn,
    description_ko: descriptionKo,
    fixed: true,
  };
}
