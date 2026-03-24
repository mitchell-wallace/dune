package gear

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"

	"claudebox/internal/dune/domain"
)

func ParseManifest(path string) ([]domain.GearSpec, error) {
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
		return nil, fmt.Errorf("failed to parse gear manifest: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	header := make(map[string]int, len(rows[0]))
	for idx, col := range rows[0] {
		header[col] = idx
	}

	specs := make([]domain.GearSpec, 0, len(rows)-1)
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

		specs = append(specs, domain.GearSpec{
			Name:           domain.GearName(name),
			Script:         fieldAt(row, header["script"]),
			Description:    fieldAt(row, header["description"]),
			EnabledModes:   modes,
			RunAs:          fieldAt(row, header["run_as"]),
			HelperCommands: helpers,
		})
	}
	return specs, nil
}

func BuildCSV(gearNames []domain.GearName, warn func(string)) string {
	seen := map[string]bool{}
	ordered := []string{}
	for _, gearName := range gearNames {
		name := string(gearName)
		if !isValidGearName(name) {
			if warn != nil {
				warn(fmt.Sprintf("Invalid gear name in dune.toml skipped for build-time install: %s", name))
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

func IndexByName(specs []domain.GearSpec) map[string]domain.GearSpec {
	index := make(map[string]domain.GearSpec, len(specs))
	for _, spec := range specs {
		index[string(spec.Name)] = spec
	}
	return index
}

func OrderedNames(specs []domain.GearSpec) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, string(spec.Name))
	}
	sort.Strings(names)
	return names
}

func DedupeRequested(gearNames []domain.GearName) []domain.GearName {
	seen := map[string]bool{}
	result := make([]domain.GearName, 0, len(gearNames))
	for _, gearName := range gearNames {
		name := string(gearName)
		if seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, gearName)
	}
	return result
}

func IsValidName(name string) bool {
	return isValidGearName(name)
}

func fieldAt(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func isValidGearName(name string) bool {
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
