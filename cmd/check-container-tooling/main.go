package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type manifest struct {
	Apt            []aptTool     `yaml:"apt"`
	NPM            []npmTool     `yaml:"npm"`
	ReleaseScripts []releaseTool `yaml:"release_scripts"`
	BinaryReleases []binaryTool  `yaml:"binary_releases"`
}

type aptTool struct {
	Name    string `yaml:"name"`
	Package string `yaml:"package"`
	Verify  string `yaml:"verify"`
	Smoke   *bool  `yaml:"smoke"`
}

type npmTool struct {
	Name       string `yaml:"name"`
	Package    string `yaml:"package"`
	VersionArg string `yaml:"version_arg"`
	Verify     string `yaml:"verify"`
	Update     bool   `yaml:"update"`
	UpdatePin  string `yaml:"update_pin"`
	Smoke      *bool  `yaml:"smoke"`
}

type releaseTool struct {
	Name          string `yaml:"name"`
	InstallScript string `yaml:"install_script"`
	VersionEnv    string `yaml:"version_env"`
	Verify        string `yaml:"verify"`
	Update        bool   `yaml:"update"`
	UpdatePin     string `yaml:"update_pin"`
	Smoke         *bool  `yaml:"smoke"`
}

type binaryTool struct {
	Name       string            `yaml:"name"`
	VersionArg string            `yaml:"version_arg"`
	Verify     string            `yaml:"verify"`
	Smoke      *bool             `yaml:"smoke"`
	Checksum   string            `yaml:"checksum"`
	Arch       map[string]string `yaml:"arch"`
}

func main() {
	root, err := repoRoot()
	if err != nil {
		fail([]string{err.Error()})
	}

	m, err := loadManifest(filepath.Join(root, "container", "base", "tooling.yaml"))
	if err != nil {
		fail([]string{err.Error()})
	}

	files, err := readFiles(root)
	if err != nil {
		fail([]string{err.Error()})
	}

	var problems []string
	problems = append(problems, checkDockerfile(m, files.dockerfile)...)
	problems = append(problems, checkBaseSmoke(m, files.baseSmoke)...)
	problems = append(problems, checkUpdateSupport(m, files.updateTools, files.toolUpdates, files.toolingData)...)
	if len(problems) > 0 {
		fail(problems)
	}
}

type trackedFiles struct {
	dockerfile  string
	baseSmoke   string
	toolUpdates string
	updateTools string
	toolingData string
}

func readFiles(root string) (trackedFiles, error) {
	paths := map[string]string{
		"dockerfile":  "Dockerfile",
		"baseSmoke":   filepath.Join("test", "smoke", "base-image.sh"),
		"toolUpdates": filepath.Join("test", "smoke", "tool-updates.sh"),
		"updateTools": filepath.Join("container", "base", "scripts", "update-tools.sh"),
		"toolingData": filepath.Join("container", "base", "scripts", "tooling-data.sh"),
	}

	data := make(map[string]string, len(paths))
	for key, rel := range paths {
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return trackedFiles{}, fmt.Errorf("read %s: %w", rel, err)
		}
		data[key] = string(raw)
	}

	return trackedFiles{
		dockerfile:  data["dockerfile"],
		baseSmoke:   data["baseSmoke"],
		toolUpdates: data["toolUpdates"],
		updateTools: data["updateTools"],
		toolingData: data["toolingData"],
	}, nil
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", fmt.Errorf("could not find repo root from %s", wd)
		}
		wd = parent
	}
}

func loadManifest(path string) (manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, fmt.Errorf("read %s: %w", path, err)
	}
	var m manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return manifest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

func checkDockerfile(m manifest, dockerfile string) []string {
	var problems []string
	for _, tool := range m.Apt {
		if !containsLineToken(dockerfile, tool.Package) {
			problems = append(problems, fmt.Sprintf("Dockerfile missing apt package for %s: %s", tool.Name, tool.Package))
		}
	}
	for _, tool := range m.NPM {
		pkg := tool.Package
		if tool.VersionArg != "" {
			pkg += "@${" + tool.VersionArg + "}"
		}
		if !strings.Contains(dockerfile, pkg) {
			problems = append(problems, fmt.Sprintf("Dockerfile missing npm package for %s: %s", tool.Name, pkg))
		}
	}
	for _, tool := range m.ReleaseScripts {
		if !strings.Contains(dockerfile, filepath.Base(tool.InstallScript)) {
			problems = append(problems, fmt.Sprintf("Dockerfile missing install script for %s: %s", tool.Name, tool.InstallScript))
		}
	}
	for _, tool := range m.BinaryReleases {
		if tool.VersionArg != "" && !strings.Contains(dockerfile, "ARG "+tool.VersionArg+"=") {
			problems = append(problems, fmt.Sprintf("Dockerfile missing version ARG for %s: %s", tool.Name, tool.VersionArg))
		}
		for goarch, value := range tool.Arch {
			if value != "" && !strings.Contains(dockerfile, value) {
				problems = append(problems, fmt.Sprintf("Dockerfile missing %s arch mapping for %s: %s", goarch, tool.Name, value))
			}
		}
	}
	return problems
}

func checkBaseSmoke(m manifest, baseSmoke string) []string {
	var problems []string
	for _, tool := range m.Apt {
		problems = appendSmokeProblem(problems, "apt", tool.Name, tool.Verify, tool.Smoke, baseSmoke)
	}
	for _, tool := range m.NPM {
		problems = appendSmokeProblem(problems, "npm", tool.Name, tool.Verify, tool.Smoke, baseSmoke)
	}
	for _, tool := range m.ReleaseScripts {
		problems = appendSmokeProblem(problems, "release", tool.Name, tool.Verify, tool.Smoke, baseSmoke)
	}
	for _, tool := range m.BinaryReleases {
		problems = appendSmokeProblem(problems, "binary", tool.Name, tool.Verify, tool.Smoke, baseSmoke)
	}
	return problems
}

func appendSmokeProblem(problems []string, kind, name, verify string, smoke *bool, baseSmoke string) []string {
	if !enabled(smoke) || strings.TrimSpace(verify) == "" {
		return problems
	}
	if !strings.Contains(baseSmoke, quoteAssert(verify)) {
		return append(problems, fmt.Sprintf("missing base-image smoke assertion for %s %s: %s", kind, name, verify))
	}
	return problems
}

func checkUpdateSupport(m manifest, updateTools, toolUpdates, toolingData string) []string {
	var problems []string
	for _, tool := range m.NPM {
		if !tool.Update {
			continue
		}
		if !strings.Contains(toolingData, tool.Name+":"+tool.Package) {
			problems = append(problems, fmt.Sprintf("tooling-data missing npm update entry for %s: %s", tool.Name, tool.Package))
		}
		problems = append(problems, checkUpdateSmoke(tool.Name, tool.UpdatePin, tool.Verify, toolUpdates)...)
	}
	for _, tool := range m.ReleaseScripts {
		if !tool.Update {
			continue
		}
		if !strings.Contains(toolingData, tool.Name+":"+tool.InstallScript+":"+tool.VersionEnv) {
			problems = append(problems, fmt.Sprintf("tooling-data missing release update entry for %s", tool.Name))
		}
		if !strings.Contains(updateTools, "version_env") {
			problems = append(problems, fmt.Sprintf("update-tools missing version env handling for %s: %s", tool.Name, tool.VersionEnv))
		}
		problems = append(problems, checkUpdateSmoke(tool.Name, tool.UpdatePin, tool.Verify, toolUpdates)...)
	}
	if !strings.Contains(updateTools, "source ") || !strings.Contains(updateTools, "tooling-data.sh") {
		problems = append(problems, "update-tools does not source tooling-data.sh")
	}
	return problems
}

func checkUpdateSmoke(name, pin, verify, toolUpdates string) []string {
	var problems []string
	if strings.TrimSpace(pin) == "" {
		problems = append(problems, fmt.Sprintf("missing update pin for %s", name))
		return problems
	}
	if !strings.Contains(toolUpdates, quoteCommand("update-tools "+name+" "+pin)) &&
		!strings.Contains(toolUpdates, quoteCommand(name)+` `+quoteCommand(pin)) {
		problems = append(problems, fmt.Sprintf("missing tool update smoke pin for %s: %s", name, pin))
	}
	if !strings.Contains(toolUpdates, quoteCommand(verify+" | grep -q "+pin)) &&
		!strings.Contains(toolUpdates, quoteCommand(verify)) {
		problems = append(problems, fmt.Sprintf("missing tool update smoke verification for %s: %s", name, verify))
	}
	return problems
}

func quoteAssert(command string) string {
	return `assert_container_command "` + command + `"`
}

func quoteCommand(command string) string {
	return `"` + command + `"`
}

func containsLineToken(text, token string) bool {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(strings.TrimRight(line, ` \`))
		for _, field := range fields {
			if field == token {
				return true
			}
		}
	}
	return false
}

func enabled(value *bool) bool {
	return value == nil || *value
}

func fail(problems []string) {
	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, problem)
	}
	os.Exit(1)
}
