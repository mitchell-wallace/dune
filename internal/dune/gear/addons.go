package gear

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"

	"claudebox/internal/dune/domain"
)

func ParseManifest(path string) ([]domain.AddonSpec, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open manifest: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse addons manifest: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	header := make(map[string]int, len(rows[0]))
	for idx, col := range rows[0] {
		header[col] = idx
	}

	specs := make([]domain.AddonSpec, 0, len(rows)-1)
	for _, row := range rows[1:] {
		name := fieldAt(row, header["name"])
		if name == "" {
			continue
		}
		modes := map[domain.Mode]bool{}
		for _, mode := range strings.Split(fieldAt(row, header["enabled_modes"]), ",") {
			switch strings.TrimSpace(mode) {
			case "std":
				modes[domain.ModeStd] = true
			case "lax":
				modes[domain.ModeLax] = true
			case "yolo":
				modes[domain.ModeYolo] = true
			case "strict":
				modes[domain.ModeStrict] = true
			}
		}

		helpers := []string{}
		helperValue := fieldAt(row, header["helper_commands"])
		if helperValue != "" && helperValue != "-" {
			for _, helper := range strings.Split(helperValue, ",") {
				helper = strings.TrimSpace(helper)
				if helper != "" {
					helpers = append(helpers, helper)
				}
			}
		}

		specs = append(specs, domain.AddonSpec{
			Name:           domain.AddonName(name),
			Script:         fieldAt(row, header["script"]),
			Description:    fieldAt(row, header["description"]),
			EnabledModes:   modes,
			RunAs:          fieldAt(row, header["run_as"]),
			HelperCommands: helpers,
		})
	}
	return specs, nil
}

func BuildCSV(addons []domain.AddonName, warn func(string)) string {
	seen := map[string]bool{}
	ordered := []string{}
	for _, addon := range addons {
		name := string(addon)
		if !isValidAddonName(name) {
			if warn != nil {
				warn(fmt.Sprintf("Invalid addon name in sand.toml skipped for build-time install: %s", name))
			}
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		ordered = append(ordered, name)
	}
	return strings.Join(ordered, ",")
}

func IndexByName(specs []domain.AddonSpec) map[string]domain.AddonSpec {
	index := make(map[string]domain.AddonSpec, len(specs))
	for _, spec := range specs {
		index[string(spec.Name)] = spec
	}
	return index
}

func OrderedNames(specs []domain.AddonSpec) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, string(spec.Name))
	}
	sort.Strings(names)
	return names
}

func DedupeRequested(addons []domain.AddonName) []domain.AddonName {
	seen := map[string]bool{}
	result := make([]domain.AddonName, 0, len(addons))
	for _, addon := range addons {
		name := string(addon)
		if seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, addon)
	}
	return result
}

func IsValidName(name string) bool {
	return isValidAddonName(name)
}

func fieldAt(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func isValidAddonName(name string) bool {
	if name == "" {
		return false
	}
	for idx, char := range name {
		if idx == 0 {
			if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')) {
				return false
			}
			continue
		}
		if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-') {
			return false
		}
	}
	return true
}
