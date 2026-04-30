package tools

import (
	"reflect"
	"testing"
)

func TestStampToolsetVersionOnOutputs(t *testing.T) {
	cases := []any{
		areaListOutput{},
		areaCreateOutput{},
		artifactRevisionsOutput{},
		artifactDiffOutput{},
		artifactProposeOutput{},
		artifactReadOutput{},
		artifactReadStateOutput{},
		artifactSearchOutput{},
		artifactTranslateOutput{},
		summarySinceOutput{},
		contextForTaskOutput{},
		harnessInstallOutput{},
		pingOutput{},
		projectCurrentOutput{},
		projectCreateOutput{},
		projectExportOutput{},
		runtimeStatusOutput{},
		scopeInFlightOutput{},
		taskAcceptanceTransitionOutput{},
		taskAssignOutput{},
		taskBulkAssignOutput{},
		taskClaimDoneOutput{},
		taskDoneCheckOutput{},
		taskQueueOutput{},
		userCurrentOutput{},
		userUpdateOutput{},
		workspaceDetectOutput{},
	}
	for _, tc := range cases {
		t.Run(reflect.TypeOf(tc).Name(), func(t *testing.T) {
			got := stampToolsetVersion(tc)
			field := reflect.ValueOf(got).FieldByName("ToolsetVersion")
			if !field.IsValid() {
				t.Fatalf("ToolsetVersion field missing")
			}
			if field.String() != ToolsetVersion() {
				t.Fatalf("ToolsetVersion = %q, want %q", field.String(), ToolsetVersion())
			}
		})
	}
}
