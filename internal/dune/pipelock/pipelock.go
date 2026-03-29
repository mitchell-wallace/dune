package pipelock

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

const (
	imageRepository = "ghcr.io/luckypipewrench/pipelock"
	// GHCR publishes semver image tags without the GitHub release's "v" prefix.
	pinnedTag = "2.0.0"
)

var (
	//go:embed pipelock.yaml.tmpl
	customizationTemplate string

	parsedTemplate = template.Must(template.New("pipelock.yaml.tmpl").Parse(customizationTemplate))
)

type TemplateData struct {
	APIAllowlist         []string
	Blocklist            []string
	MaxRequestsPerMinute int
}

func PinnedTag() string {
	return pinnedTag
}

func ImageRef() string {
	return imageRepository + ":" + pinnedTag
}

func GenerateConfigCommand() []string {
	return []string{
		"docker",
		"run",
		"--rm",
		ImageRef(),
		"generate",
		"config",
		"--preset",
		"balanced",
	}
}

func DefaultTemplateData() TemplateData {
	return TemplateData{
		APIAllowlist: []string{
			"*.anthropic.com",
			"*.openai.com",
			"*.googleapis.com",
			"accounts.google.com",
			"oauth2.googleapis.com",
			"chatgpt.com",
			"registry.npmjs.org",
			"pypi.org",
			"files.pythonhosted.org",
			"proxy.golang.org",
			"crates.io",
			"mcp.grep.app",
			"mcp.context7.com",
			"mcp.exa.ai",
		},
		Blocklist: []string{
			"*.pastebin.com",
			"*.hastebin.com",
			"*.transfer.sh",
			"file.io",
			"requestbin.net",
		},
		MaxRequestsPerMinute: 60,
	}
}

func RenderCustomizationYAML() ([]byte, error) {
	var rendered bytes.Buffer
	if err := parsedTemplate.Execute(&rendered, DefaultTemplateData()); err != nil {
		return nil, fmt.Errorf("render pipelock customization template: %w", err)
	}
	return rendered.Bytes(), nil
}

func ApplyCustomizations(baseline []byte) ([]byte, error) {
	var base map[string]any
	if err := yaml.Unmarshal(baseline, &base); err != nil {
		return nil, fmt.Errorf("parse baseline pipelock config: %w", err)
	}

	customizationYAML, err := RenderCustomizationYAML()
	if err != nil {
		return nil, err
	}

	var overlay map[string]any
	if err := yaml.Unmarshal(customizationYAML, &overlay); err != nil {
		return nil, fmt.Errorf("parse pipelock customization template: %w", err)
	}

	existingAllowlist := getStringSlice(base, "api_allowlist")
	existingBlocklist := getStringSlice(base, "fetch_proxy", "monitoring", "blocklist")

	deepMerge(base, overlay)

	setValue(base, mergeUnique(existingAllowlist, getStringSlice(overlay, "api_allowlist")), "api_allowlist")
	setValue(
		base,
		mergeUnique(existingBlocklist, getStringSlice(overlay, "fetch_proxy", "monitoring", "blocklist")),
		"fetch_proxy",
		"monitoring",
		"blocklist",
	)

	rendered, err := yaml.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("render customized pipelock config: %w", err)
	}

	return rendered, nil
}

func deepMerge(dst, src map[string]any) {
	for key, value := range src {
		srcMap, srcIsMap := value.(map[string]any)
		if !srcIsMap {
			dst[key] = value
			continue
		}

		dstMap, dstIsMap := dst[key].(map[string]any)
		if !dstIsMap {
			dstMap = map[string]any{}
			dst[key] = dstMap
		}

		deepMerge(dstMap, srcMap)
	}
}

func getStringSlice(root map[string]any, path ...string) []string {
	value, ok := getValue(root, path...)
	if !ok {
		return nil
	}

	rawItems, ok := value.([]any)
	if !ok {
		return nil
	}

	items := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		text, ok := item.(string)
		if !ok {
			continue
		}
		items = append(items, text)
	}
	return items
}

func getValue(root map[string]any, path ...string) (any, bool) {
	current := any(root)
	for _, segment := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}

		next, ok := mapping[segment]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func setValue(root map[string]any, value any, path ...string) {
	current := root
	for idx, segment := range path {
		if idx == len(path)-1 {
			current[segment] = value
			return
		}

		next, ok := current[segment].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[segment] = next
		}
		current = next
	}
}

func mergeUnique(existing, desired []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(desired))
	merged := make([]string, 0, len(existing)+len(desired))

	appendUnique := func(items []string) {
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			merged = append(merged, item)
		}
	}

	appendUnique(existing)
	appendUnique(desired)

	return merged
}
