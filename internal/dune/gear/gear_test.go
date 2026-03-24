package gear

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"claudebox/internal/dune/domain"
)

func TestBuildCSVDedupesAndSkipsInvalid(t *testing.T) {
	t.Parallel()

	var warnings []string
	got := BuildCSV([]domain.GearName{"add-go", "invalid/name", "add-go", "add-rust"}, func(message string) {
		warnings = append(warnings, message)
	})

	if got != "add-go,add-rust" {
		t.Fatalf("unexpected csv: %s", got)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected one warning, got %d", len(warnings))
	}
}

func TestParseManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.tsv")
	content := "name\tscript\tdescription\tenabled_modes\trun_as\thelper_commands\nadd-go\tadd-go.sh\tGo\tstd,lax\troot\tgo\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	specs, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest returned error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if !reflect.DeepEqual(specs[0].HelperCommands, []string{"go"}) {
		t.Fatalf("unexpected helper commands: %#v", specs[0].HelperCommands)
	}
}
