package cmd

import (
	"testing"

	"ctx/internal/model"
)

func TestParseSetArgsPath(t *testing.T) {
	got, err := parseSetArgs([]string{"s1", "SPEC", "--path", "./openapi.yaml"})
	if err != nil {
		t.Fatalf("parseSetArgs error: %v", err)
	}
	if got.sessionID != "s1" || got.key != "SPEC" || got.value != "./openapi.yaml" || got.valueType != model.ValueTypeFileRef {
		t.Fatalf("parseSetArgs = %#v, want file_ref", got)
	}
}
