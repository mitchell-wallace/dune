package config

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"claudebox/internal/dune/domain"
)

var VersionKeys = []string{
	"python_version",
	"uv_version",
	"go_version",
	"rust_version",
}

var ScalarKeys = append([]string{"profile", "mode", "workspace_mode"}, VersionKeys...)

var allowedKeys = func() map[string]struct{} {
	keys := make(map[string]struct{}, len(ScalarKeys)+2)
	for _, key := range ScalarKeys {
		keys[key] = struct{}{}
	}
	keys["gear"] = struct{}{}
	keys["addons"] = struct{}{}
	return keys
}()

func DefaultConfig() domain.DuneConfig {
	return domain.DuneConfig{
		Profile:       domain.Profile("0"),
		Mode:          domain.ModeStd,
		WorkspaceMode: domain.WorkspaceModeMount,
		Addons:        []domain.AddonName{},
	}
}

func NormalizeProfile(raw string) (domain.Profile, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if len(value) != 1 {
		return "", false
	}
	char := value[0]
	if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'z') {
		return domain.Profile(value), true
	}
	return "", false
}

func IsProfileToken(raw string) bool {
	_, ok := NormalizeProfile(raw)
	return ok
}

func CanonicalizeMode(raw string) (domain.Mode, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "std", "standard":
		return domain.ModeStd, true
	case "lax":
		return domain.ModeLax, true
	case "yolo":
		return domain.ModeYolo, true
	case "strict":
		return domain.ModeStrict, true
	default:
		return "", false
	}
}

func NormalizeWorkspaceMode(raw string) (domain.WorkspaceMode, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "mount":
		return domain.WorkspaceModeMount, true
	case "copy":
		return domain.WorkspaceModeCopy, true
	default:
		return "", false
	}
}

func Load(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return map[string]any{}, nil
	}

	var data map[string]any
	if err := toml.Unmarshal(content, &data); err != nil {
		return nil, err
	}
	if data == nil {
		return map[string]any{}, nil
	}
	return data, nil
}

func Write(path string, data map[string]any) error {
	rendered, err := toml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to render dune.toml: %w", err)
	}
	if len(rendered) == 0 || rendered[len(rendered)-1] != '\n' {
		rendered = append(rendered, '\n')
	}
	return os.WriteFile(path, rendered, 0o644)
}

func Parse(data map[string]any) (domain.DuneConfig, []string, error) {
	cfg := DefaultConfig()
	warnings := []string{}

	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := allowedKeys[key]; !ok {
			cfg.UnknownKeys = append(cfg.UnknownKeys, key)
			warnings = append(warnings, fmt.Sprintf("Unknown key in dune.toml ignored: %s", key))
		}
	}

	for _, key := range ScalarKeys {
		value, ok := data[key]
		if !ok || value == nil {
			continue
		}
		strValue, ok := value.(string)
		if !ok {
			return cfg, warnings, fmt.Errorf("invalid dune.toml (%s): expected string", key)
		}

		switch key {
		case "profile":
			profile, valid := NormalizeProfile(strValue)
			if !valid {
				return cfg, warnings, fmt.Errorf("invalid profile in dune.toml: %q", strValue)
			}
			cfg.Profile = profile
		case "mode":
			mode, valid := CanonicalizeMode(strValue)
			if !valid {
				return cfg, warnings, fmt.Errorf("invalid mode in dune.toml: %q", strValue)
			}
			cfg.Mode = mode
		case "workspace_mode":
			workspaceMode, valid := NormalizeWorkspaceMode(strValue)
			if !valid {
				return cfg, warnings, fmt.Errorf("invalid workspace_mode in dune.toml: %q", strValue)
			}
			cfg.WorkspaceMode = workspaceMode
		case "python_version":
			cfg.PythonVersion = strings.TrimSpace(strValue)
		case "uv_version":
			cfg.UVVersion = strings.TrimSpace(strValue)
		case "go_version":
			cfg.GoVersion = strings.TrimSpace(strValue)
		case "rust_version":
			cfg.RustVersion = strings.TrimSpace(strValue)
		}
	}

	if _, hasGear := data["gear"]; hasGear {
		if _, hasAddons := data["addons"]; hasAddons {
			warnings = append(warnings, "Both gear and addons are set in dune.toml; gear takes precedence.")
		}
	}
	items, err := gearListValue(data)
	if err != nil {
		return cfg, warnings, err
	}
	if items != nil {
		cfg.Addons = make([]domain.AddonName, 0, len(items))
		for _, item := range items {
			cfg.Addons = append(cfg.Addons, domain.AddonName(item))
		}
	}

	return cfg, warnings, nil
}

func ValidateExistingData(data map[string]any) []string {
	var warnings []string
	for _, key := range ScalarKeys {
		value, ok := data[key]
		if !ok || value == nil {
			continue
		}
		if _, ok := value.(string); !ok {
			warnings = append(warnings, fmt.Sprintf("Existing dune.toml key %q is not a string; it will be kept until overwritten.", key))
		}
	}
	if _, err := gearListValue(data); err != nil {
		warnings = append(warnings, err.Error())
	}
	return warnings
}

func ExistingAddons(data map[string]any) map[string]bool {
	result := map[string]bool{}
	items, err := gearListValue(data)
	if err != nil || items == nil {
		return result
	}
	for _, item := range items {
		result[item] = true
	}
	return result
}

func ExistingVersions(data map[string]any) map[string]string {
	result := map[string]string{}
	for _, key := range VersionKeys {
		value, ok := data[key]
		if !ok || value == nil {
			continue
		}
		if strValue, ok := value.(string); ok && strValue != "" {
			result[key] = strValue
		}
	}
	return result
}

func UpdateData(data map[string]any, cfg domain.DuneConfig, configureVersions bool, versionUpdates map[string]string) {
	data["profile"] = string(cfg.Profile)
	data["mode"] = string(cfg.Mode)
	data["workspace_mode"] = string(cfg.WorkspaceMode)
	data["gear"] = addonStrings(cfg.Addons)
	delete(data, "addons")

	if !configureVersions {
		return
	}

	for _, key := range VersionKeys {
		value := strings.TrimSpace(versionUpdates[key])
		if value == "" {
			delete(data, key)
			continue
		}
		data[key] = value
	}
}

func RenderParseLines(data map[string]any) ([]string, error) {
	lines := []string{}
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := allowedKeys[key]; !ok {
			lines = append(lines, fmt.Sprintf("unknown\t%s\t", key))
		}
	}

	for _, key := range ScalarKeys {
		value, ok := data[key]
		if !ok || value == nil {
			continue
		}
		strValue, ok := value.(string)
		if !ok {
			return lines, fmt.Errorf("error\t%s\texpected string", key)
		}
		lines = append(lines, fmt.Sprintf("scalar\t%s\t%s", key, strValue))
	}

	items, err := gearListValue(data)
	if err != nil {
		return lines, err
	}
	if items == nil {
		return lines, nil
	}
	for _, value := range items {
		lines = append(lines, fmt.Sprintf("addon\t%s\t", value))
	}
	return lines, nil
}

func addonStrings(addons []domain.AddonName) []string {
	values := make([]string, 0, len(addons))
	for _, addon := range addons {
		values = append(values, string(addon))
	}
	return values
}

func gearListValue(data map[string]any) ([]string, error) {
	var key string
	switch {
	case data["gear"] != nil:
		key = "gear"
	case data["addons"] != nil:
		key = "addons"
	default:
		return nil, nil
	}

	items, ok := data[key].([]any)
	if !ok {
		return nil, fmt.Errorf("existing %s key is not a list; the wizard will replace it.", key)
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("invalid dune.toml (%s): expected array of strings", key)
		}
		values = append(values, value)
	}
	return values, nil
}
