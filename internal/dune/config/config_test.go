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
		"gear\tadd-go\t",
		"gear\tadd-postgres\t",
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

	UpdateData(data, domain.DuneConfig{
		Profile:       domain.Profile("a"),
		Mode:          domain.ModeLax,
		WorkspaceMode: domain.WorkspaceModeMount,
		Gear:          []domain.GearName{"add-go"},
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
}

func TestUpdateDataAppliesVersionEdits(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"go_version": "1.25.4",
		"uv_version": "0.10.4",
	}

	UpdateData(data, domain.DuneConfig{
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

func TestParseBeadsAuto(t *testing.T) {
	t.Parallel()
	cfg, _, err := Parse(map[string]any{"beads": "auto"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Beads != "auto" {
		t.Fatalf("expected beads=auto, got %q", cfg.Beads)
	}
}

func TestParseBeadsTrue(t *testing.T) {
	t.Parallel()
	cfg, _, err := Parse(map[string]any{"beads": "true"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Beads != "true" {
		t.Fatalf("expected beads=true, got %q", cfg.Beads)
	}
}

func TestParseBeadsAbsent(t *testing.T) {
	t.Parallel()
	cfg, _, err := Parse(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Beads != "" {
		t.Fatalf("expected beads empty, got %q", cfg.Beads)
	}
}

func TestParseBeadsInvalid(t *testing.T) {
	t.Parallel()
	_, _, err := Parse(map[string]any{"beads": "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid beads value")
	}
}

func TestUpdateDataWritesBeads(t *testing.T) {
	t.Parallel()
	data := map[string]any{}
	UpdateData(data, domain.DuneConfig{
		Profile:       domain.Profile("0"),
		Mode:          domain.ModeStd,
		WorkspaceMode: domain.WorkspaceModeMount,
		Beads:         "auto",
	}, false, nil)
	if got := data["beads"]; got != "auto" {
		t.Fatalf("expected beads=auto in data, got %v", got)
	}
}

func TestUpdateDataOmitsEmptyBeads(t *testing.T) {
	t.Parallel()
	data := map[string]any{}
	UpdateData(data, domain.DuneConfig{
		Profile:       domain.Profile("0"),
		Mode:          domain.ModeStd,
		WorkspaceMode: domain.WorkspaceModeMount,
	}, false, nil)
	if _, ok := data["beads"]; ok {
		t.Fatal("expected beads absent from data when empty")
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
