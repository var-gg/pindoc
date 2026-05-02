package preflight

type ForbiddenVerb struct {
	Key           string
	Canonical     string
	Variants      []string
	ExampleBefore string
	ExampleAfter  string
}

var ForbiddenAcceptanceVerbs = []ForbiddenVerb{
	{
		Key:           "research",
		Canonical:     "조사한다",
		Variants:      []string{"조사한다", "조사했다", "조사합니다"},
		ExampleBefore: "레거시 owner_id 사용처를 조사한다.",
		ExampleAfter:  "레거시 owner_id 사용처 목록이 report에 존재한다.",
	},
	{
		Key:           "organize",
		Canonical:     "정리한다",
		Variants:      []string{"정리한다", "정리했다", "정리합니다"},
		ExampleBefore: "릴리스 리스크를 정리한다.",
		ExampleAfter:  "릴리스 리스크 표가 Outcome 섹션에 포함된다.",
	},
	{
		Key:           "review",
		Canonical:     "검토한다",
		Variants:      []string{"검토한다", "검토했다", "검토합니다"},
		ExampleBefore: "API 응답 스키마를 검토한다.",
		ExampleAfter:  "API 응답 스키마 regression test가 통과한다.",
	},
	{
		Key:           "observe",
		Canonical:     "관측한다",
		Variants:      []string{"관측한다", "관측했다", "관측합니다"},
		ExampleBefore: "Reader 동작을 관측한다.",
		ExampleAfter:  "Reader smoke test 결과가 TC artifact에 기록된다.",
	},
	{
		Key:           "update",
		Canonical:     "갱신한다",
		Variants:      []string{"갱신한다", "갱신했다", "갱신합니다"},
		ExampleBefore: "템플릿 문구를 갱신한다.",
		ExampleAfter:  "_template_task revision 2에 first-mention 예시가 존재한다.",
	},
	{
		Key:           "classify",
		Canonical:     "분류한다",
		Variants:      []string{"분류한다", "분류했다", "분류합니다"},
		ExampleBefore: "기존 Task를 분류한다.",
		ExampleAfter:  "기존 Task 분류 결과 CSV가 생성된다.",
	},
	{
		Key:           "inspect",
		Canonical:     "살펴본다",
		Variants:      []string{"살펴본다", "살펴봤다", "살펴보았다", "살펴봅니다"},
		ExampleBefore: "Sidebar 레이아웃을 살펴본다.",
		ExampleAfter:  "Sidebar 레이아웃 QA screenshot이 report에 첨부된다.",
	},
	{
		Key:           "confirm",
		Canonical:     "확인한다",
		Variants:      []string{"확인한다", "확인했다", "확인합니다"},
		ExampleBefore: "기존 라우팅이 동작하는지 확인한다.",
		ExampleAfter:  "기존 /p/{project}/wiki/{slug} 라우팅 regression test가 통과한다.",
	},
	{
		Key:           "identify",
		Canonical:     "식별한다",
		Variants:      []string{"식별한다", "식별했다", "식별합니다"},
		ExampleBefore: "중복 정보를 식별한다.",
		ExampleAfter:  "중복 정보 제거 후보 5건이 Analysis artifact에 기록된다.",
	},
}

func SuggestedRewriteActions(limit int) []string {
	if limit <= 0 || limit > len(ForbiddenAcceptanceVerbs) {
		limit = len(ForbiddenAcceptanceVerbs)
	}
	out := []string{"Rewrite acceptance criteria as verifiable outcomes, not investigation actions."}
	for _, rule := range ForbiddenAcceptanceVerbs[:limit] {
		out = append(out, "Example: `"+rule.ExampleBefore+"` -> `"+rule.ExampleAfter+"`")
	}
	return out
}
