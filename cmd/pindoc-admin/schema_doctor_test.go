package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/var-gg/pindoc/internal/pindoc/db"
)

func TestWriteSchemaHealthHuman(t *testing.T) {
	health := db.MigrationHealth{
		Healthy:               false,
		LedgerPresent:         true,
		ChecksumColumnPresent: true,
		KnownCount:            68,
		AppliedCount:          69,
		Issues: []db.MigrationIssue{{
			Code:        db.MigrationUnknownAppliedCode,
			MigrationID: "0063_note_threads.sql",
			Detail:      "database contains a migration that is not embedded in this binary",
		}},
	}
	var out bytes.Buffer
	if err := writeSchemaHealth(&out, health, false); err != nil {
		t.Fatalf("writeSchemaHealth() error = %v", err)
	}
	for _, want := range []string{"schema: drift", "known=68 applied=69", db.MigrationUnknownAppliedCode, "0063_note_threads.sql"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output %q missing %q", out.String(), want)
		}
	}
}

func TestWriteSchemaHealthJSON(t *testing.T) {
	health := db.MigrationHealth{Healthy: true, LedgerPresent: true, ChecksumColumnPresent: true, KnownCount: 68, AppliedCount: 68}
	var out bytes.Buffer
	if err := writeSchemaHealth(&out, health, true); err != nil {
		t.Fatalf("writeSchemaHealth() error = %v", err)
	}
	for _, want := range []string{`"healthy": true`, `"known_count": 68`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("JSON output %q missing %q", out.String(), want)
		}
	}
}
