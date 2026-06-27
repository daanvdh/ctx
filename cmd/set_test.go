package cmd

import (
	"strings"
	"testing"

	"ctx/internal/model"
)

func TestParseSetArgsRef(t *testing.T) {
	got, err := parseSetArgs([]string{"s1", "SPEC", "--ref", "./openapi.yaml"})
	if err != nil {
		t.Fatalf("parseSetArgs error: %v", err)
	}
	if got.sessionID != "s1" || got.key != "SPEC" || got.value != "./openapi.yaml" || got.valueType != model.ValueTypeFileRef {
		t.Fatalf("parseSetArgs = %#v, want file_ref", got)
	}
}

func TestParseSetArgsRejectsOldFileRefFlag(t *testing.T) {
	_, err := parseSetArgs([]string{"SPEC", "--file-ref", "./openapi.yaml"})
	if err == nil || !strings.Contains(err.Error(), "use --ref") {
		t.Fatalf("parseSetArgs error = %v, want --ref guidance", err)
	}
}
