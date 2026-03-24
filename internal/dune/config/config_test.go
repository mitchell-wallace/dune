package config

import (
	"reflect"
	"testing"

	"claudebox/internal/dune/domain"
)

func TestRenderParseLines(t *testing.T) {
	t.Parallel()

	lines, err := RenderParseLines(map[string]any{
		"profile":         "a",
		"mode":            "strict",
		"workspace_mode":  "copy",
		"go_version":      "1.26.0",
		"gear":            []any{"add-go", "add-postgres"},
		"unexpected_key":  42,
		"another_unknown": true,
	})
	if err != nil {
		t.Fatalf("RenderParseLines returned error: %v", err)
	}

	want := []string{
		"unknown\tanother_unknown\t",
		"unknown\tunexpected_key\t",
		"scalar\tprofile\ta",
		"scalar\tmode\tstrict",
		"scalar\tworkspace_mode\tcopy",
		"scalar\tgo_version\t1.26.0",
		"addon\tadd-go\t",
		"addon\tadd-postgres\t",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("unexpected parse lines:\n got %#v\nwant %#v", lines, want)
	}
}

func TestUpdateDataKeepsVersionsWhenUnchanged(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"profile":     "0",
		"mode":        "std",
		"go_version":  "1.25.4",
		"uv_version":  "0.10.4",
		"custom_flag": true,
	}

	UpdateData(data, domain.SandConfig{
		Profile:       domain.Profile("a"),
		Mode:          domain.ModeLax,
		WorkspaceMode: domain.WorkspaceModeMount,
		Addons:        []domain.AddonName{"add-go"},
	}, false, nil)

	if got := data["go_version"]; got != "1.25.4" {
		t.Fatalf("go_version changed unexpectedly: %v", got)
	}
	if got := data["uv_version"]; got != "0.10.4" {
		t.Fatalf("uv_version changed unexpectedly: %v", got)
	}
	if got := data["custom_flag"]; got != true {
		t.Fatalf("custom_flag changed unexpectedly: %v", got)
	}
	if got := data["gear"]; !reflect.DeepEqual(got, []string{"add-go"}) {
		t.Fatalf("gear not written correctly: %#v", got)
	}
	if _, ok := data["addons"]; ok {
		t.Fatalf("legacy addons key should have been removed")
	}
}

func TestUpdateDataAppliesVersionEdits(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"go_version": "1.25.4",
		"uv_version": "0.10.4",
	}

	UpdateData(data, domain.SandConfig{
		Profile:       domain.Profile("b"),
		Mode:          domain.ModeStd,
		WorkspaceMode: domain.WorkspaceModeCopy,
	}, true, map[string]string{
		"go_version": "1.26.0",
		"uv_version": "",
	})

	if got := data["go_version"]; got != "1.26.0" {
		t.Fatalf("go_version not updated: %v", got)
	}
	if _, ok := data["uv_version"]; ok {
		t.Fatalf("uv_version should have been removed")
	}
}

func TestParseRejectsUnsupportedVersionKeyTypes(t *testing.T) {
	t.Parallel()

	_, _, err := Parse(map[string]any{
		"bun_version": "1.0.0",
		"go_version":  123,
	})
	if err == nil {
		t.Fatal("expected parse error")
	}
}
