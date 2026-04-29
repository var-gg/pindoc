package tools

import (
	"reflect"
	"strings"
	"testing"
)

func TestTaskMetaPriorityDescriptionIncludesMeanings(t *testing.T) {
	field, ok := reflect.TypeOf(TaskMetaInput{}).FieldByName("Priority")
	if !ok {
		t.Fatalf("TaskMetaInput.Priority field missing")
	}
	schema := field.Tag.Get("jsonschema")
	for _, want := range []string{"p0 release blocker", "p1 must close before release", "p2 next round", "p3 backlog"} {
		if !strings.Contains(schema, want) {
			t.Fatalf("priority jsonschema tag %q missing %q", schema, want)
		}
	}
	if !strings.Contains(schema, "Project-specific priority policy wins") {
		t.Fatalf("priority jsonschema tag %q missing project-specific policy note", schema)
	}
}
