package version

import (
	"strings"
	"testing"
)

func TestSbxTemplateRef(t *testing.T) {
	ref := SbxTemplateRef()
	const repo = "ghcr.io/mitchell-wallace/dune-sbx"

	if !strings.HasPrefix(ref, repo+":") {
		t.Fatalf("SbxTemplateRef() = %q, want repo %q", ref, repo)
	}
	if version := strings.TrimPrefix(ref, repo+":"); strings.TrimSpace(version) == "" {
		t.Fatalf("SbxTemplateRef() = %q, want non-empty version", ref)
	}
}
