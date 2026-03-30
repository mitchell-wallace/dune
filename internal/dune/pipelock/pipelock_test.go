package pipelock

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPinnedImageMetadata(t *testing.T) {
	t.Parallel()

	if got := PinnedTag(); got != "2.0.0" {
		t.Fatalf("PinnedTag() = %q, want %q", got, "2.0.0")
	}

	if got := ImageRef(); got != "ghcr.io/luckypipewrench/pipelock:2.0.0" {
		t.Fatalf("ImageRef() = %q", got)
	}

	want := []string{
		"docker",
		"run",
		"--rm",
		"ghcr.io/luckypipewrench/pipelock:2.0.0",
		"generate",
		"config",
		"--preset",
		"balanced",
	}
	got := GenerateConfigCommand()
	if len(got) != len(want) {
		t.Fatalf("GenerateConfigCommand() length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("GenerateConfigCommand()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRenderCustomizationYAML(t *testing.T) {
	t.Parallel()

	rendered, err := RenderCustomizationYAML()
	if err != nil {
		t.Fatalf("RenderCustomizationYAML() error = %v", err)
	}

	cfg := parseYAMLMap(t, rendered)
	assertEqual(t, getNestedValue(t, cfg, "enforce"), true)
	assertEqual(t, getNestedValue(t, cfg, "response_scanning", "enabled"), true)
	assertEqual(t, getNestedValue(t, cfg, "response_scanning", "action"), "warn")
	assertEqual(t, getNestedValue(t, cfg, "dlp", "include_defaults"), true)
	assertEqual(t, getNestedValue(t, cfg, "fetch_proxy", "monitoring", "max_requests_per_minute"), 60)

	assertContains(t, getStringSliceAt(t, cfg, "api_allowlist"), "*.googleapis.com")
	assertContains(t, getStringSliceAt(t, cfg, "api_allowlist"), "mcp.context7.com")
	assertContains(t, getStringSliceAt(t, cfg, "fetch_proxy", "monitoring", "blocklist"), "file.io")
	assertContains(t, getStringSliceAt(t, cfg, "fetch_proxy", "monitoring", "blocklist"), "requestbin.net")
}

func TestApplyCustomizations(t *testing.T) {
	t.Parallel()

	baselinePath := filepath.Join("testdata", "balanced-2.0.0.yaml")
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", baselinePath, err)
	}

	rendered, err := ApplyCustomizations(baseline)
	if err != nil {
		t.Fatalf("ApplyCustomizations() error = %v", err)
	}

	cfg := parseYAMLMap(t, rendered)
	assertEqual(t, getNestedValue(t, cfg, "version"), 1)
	assertEqual(t, getNestedValue(t, cfg, "mode"), "balanced")
	assertEqual(t, getNestedValue(t, cfg, "enforce"), true)
	assertEqual(t, getNestedValue(t, cfg, "response_scanning", "enabled"), true)
	assertEqual(t, getNestedValue(t, cfg, "response_scanning", "action"), "warn")
	assertEqual(t, getNestedValue(t, cfg, "dlp", "include_defaults"), true)
	assertEqual(t, getNestedValue(t, cfg, "fetch_proxy", "monitoring", "max_requests_per_minute"), 60)
	assertEqual(t, getNestedValue(t, cfg, "logging", "format"), "json")
	assertEqual(t, getNestedValue(t, cfg, "logging", "output"), "stdout")

	allowlist := getStringSliceAt(t, cfg, "api_allowlist")
	assertContains(t, allowlist, "github.com")
	assertContains(t, allowlist, "*.googleapis.com")
	assertContains(t, allowlist, "accounts.google.com")
	assertContains(t, allowlist, "mcp.exa.ai")

	blocklist := getStringSliceAt(t, cfg, "fetch_proxy", "monitoring", "blocklist")
	assertContains(t, blocklist, "*.requestbin.com")
	assertContains(t, blocklist, "requestbin.net")
	assertContains(t, blocklist, "*.file.io")
	assertContains(t, blocklist, "file.io")

	if got := getNestedValue(t, cfg, "request_body_scanning", "enabled"); got != true {
		t.Fatalf("request_body_scanning.enabled = %#v, want true", got)
	}
}

func TestApplyCustomizationsProducesStructuredYAML(t *testing.T) {
	t.Parallel()

	baselinePath := filepath.Join("testdata", "balanced-2.0.0.yaml")
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", baselinePath, err)
	}

	rendered, err := ApplyCustomizations(baseline)
	if err != nil {
		t.Fatalf("ApplyCustomizations() error = %v", err)
	}

	var cfg struct {
		Version          int    `yaml:"version"`
		Mode             string `yaml:"mode"`
		Enforce          bool   `yaml:"enforce"`
		ResponseScanning struct {
			Enabled bool   `yaml:"enabled"`
			Action  string `yaml:"action"`
		} `yaml:"response_scanning"`
		DLP struct {
			IncludeDefaults bool `yaml:"include_defaults"`
		} `yaml:"dlp"`
		Logging struct {
			Format string `yaml:"format"`
			Output string `yaml:"output"`
		} `yaml:"logging"`
		APIAllowlist []string `yaml:"api_allowlist"`
		FetchProxy   struct {
			Monitoring struct {
				MaxRequestsPerMinute int      `yaml:"max_requests_per_minute"`
				Blocklist            []string `yaml:"blocklist"`
			} `yaml:"monitoring"`
		} `yaml:"fetch_proxy"`
	}

	if err := yaml.Unmarshal(rendered, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal() structured error = %v", err)
	}

	assertEqual(t, cfg.Version, 1)
	assertEqual(t, cfg.Mode, "balanced")
	assertEqual(t, cfg.Enforce, true)
	assertEqual(t, cfg.ResponseScanning.Enabled, true)
	assertEqual(t, cfg.ResponseScanning.Action, "warn")
	assertEqual(t, cfg.DLP.IncludeDefaults, true)
	assertEqual(t, cfg.Logging.Format, "json")
	assertEqual(t, cfg.Logging.Output, "stdout")
	assertEqual(t, cfg.FetchProxy.Monitoring.MaxRequestsPerMinute, 60)
	assertContains(t, cfg.APIAllowlist, "*.anthropic.com")
	assertContains(t, cfg.FetchProxy.Monitoring.Blocklist, "*.transfer.sh")
}

func parseYAMLMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var cfg map[string]any
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	return cfg
}

func getNestedValue(t *testing.T, root map[string]any, path ...string) any {
	t.Helper()

	current := any(root)
	for _, segment := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("path %v does not resolve to a map", path)
		}
		next, ok := mapping[segment]
		if !ok {
			t.Fatalf("missing path %v", path)
		}
		current = next
	}
	return current
}

func getStringSliceAt(t *testing.T, root map[string]any, path ...string) []string {
	t.Helper()

	rawItems, ok := getNestedValue(t, root, path...).([]any)
	if !ok {
		t.Fatalf("path %v does not resolve to a string slice", path)
	}

	items := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("path %v contains non-string item %#v", path, item)
		}
		items = append(items, text)
	}
	return items
}

func assertContains(t *testing.T, items []string, want string) {
	t.Helper()

	for _, item := range items {
		if item == want {
			return
		}
	}
	t.Fatalf("slice %v does not contain %q", items, want)
}

func assertEqual(t *testing.T, got, want any) {
	t.Helper()

	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
