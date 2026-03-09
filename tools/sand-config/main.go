package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	toml "github.com/pelletier/go-toml/v2"
)

var versionKeys = []string{
	"python_version",
	"uv_version",
	"go_version",
	"rust_version",
}

var scalarKeys = append([]string{"profile", "mode", "workspace_mode"}, versionKeys...)

var allowedKeys = func() map[string]struct{} {
	keys := make(map[string]struct{}, len(scalarKeys)+1)
	for _, key := range scalarKeys {
		keys[key] = struct{}{}
	}
	keys["addons"] = struct{}{}
	return keys
}()

var modeOptions = []option{
	{Value: "std", Description: "firewall enabled, curated addons available"},
	{Value: "lax", Description: "firewall enabled, passwordless sudo"},
	{Value: "yolo", Description: "firewall disabled, passwordless sudo"},
	{Value: "strict", Description: "firewall enabled, addons disabled, workspace copied (not mounted)"},
}

var workspaceModeOptions = []option{
	{Value: "mount", Description: "bind-mount workspace from host (read-write, default)"},
	{Value: "copy", Description: "copy workspace into container (host filesystem unchanged, use git to sync)"},
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	boxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("6")).Padding(1, 2)
	selected   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	muted      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	warning    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
)

type option struct {
	Value       string
	Description string
}

type addon struct {
	Name         string
	Description  string
	EnabledModes map[string]bool
}

type wizardOptions struct {
	Directory string
	Manifest  string
	RepoRoot  string
}

type parseOptions struct {
	Path string
}

type cliCommand int

const (
	commandWizard cliCommand = iota
	commandParse
)

type exitError struct {
	Code int
	Err  error
}

func (e *exitError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *exitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type wizardConfig struct {
	RepoRoot         string
	TargetPath       string
	ProfileWarning   string
	Discovered       []string
	Warnings         []string
	Addons           []addon
	ExistingProfile  string
	ExistingMode     string
	ExistingWSMode   string
	ExistingAddons   map[string]bool
	ExistingVersions map[string]string
}

type wizardResult struct {
	Cancelled         bool
	WriteChanges      bool
	Profile           string
	Mode              string
	WorkspaceMode     string
	SelectedAddons    []string
	ConfigureVersions bool
	VersionUpdates    map[string]string
}

type wizardStep int

const (
	stepIntro wizardStep = iota
	stepProfileSelect
	stepProfileCustom
	stepModeSelect
	stepWorkspaceMode
	stepAddons
	stepStrictAddons
	stepVersionConfirm
	stepVersionInput
	stepReview
)

type wizardModel struct {
	cfg wizardConfig

	step wizardStep

	cursor      int
	addonCursor int
	versionIdx  int

	textInput textinput.Model
	message   string

	profile           string
	mode              string
	workspaceMode     string
	selectedAddons    map[string]bool
	configureVersions bool
	versionUpdates    map[string]string

	cancelled    bool
	writeChanges bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			if exitErr.Err != nil {
				fmt.Fprintln(os.Stderr, exitErr.Err)
			}
			os.Exit(exitErr.Code)
		}

		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	command, wizardArgs, parserArgs, err := parseCLI(argv)
	if err != nil {
		return err
	}

	switch command {
	case commandParse:
		return runParse(parserArgs)
	case commandWizard:
		return runWizard(wizardArgs)
	default:
		return &exitError{Code: 1, Err: fmt.Errorf("unknown command")}
	}
}

func parseCLI(argv []string) (cliCommand, wizardOptions, parseOptions, error) {
	if len(argv) > 0 && argv[0] == "parse" {
		fs := flag.NewFlagSet("sand-config parse", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		var opts parseOptions
		fs.StringVar(&opts.Path, "path", "", "Path to sand.toml.")
		if err := fs.Parse(argv[1:]); err != nil {
			return 0, wizardOptions{}, parseOptions{}, &exitError{Code: 2, Err: err}
		}
		if opts.Path == "" {
			return 0, wizardOptions{}, parseOptions{}, &exitError{Code: 2, Err: fmt.Errorf("missing required --path")}
		}
		return commandParse, wizardOptions{}, opts, nil
	}

	fs := flag.NewFlagSet("sand-config", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var opts wizardOptions
	fs.StringVar(&opts.Directory, "directory", ".", "Workspace directory to inspect.")
	fs.StringVar(&opts.Manifest, "manifest", "", "Path to addons manifest.tsv file.")
	fs.StringVar(&opts.RepoRoot, "repo-root", "", "Internal/testing override for repository root.")
	if err := fs.Parse(argv); err != nil {
		return 0, wizardOptions{}, parseOptions{}, &exitError{Code: 2, Err: err}
	}
	if opts.Manifest == "" {
		return 0, wizardOptions{}, parseOptions{}, &exitError{Code: 2, Err: fmt.Errorf("missing required --manifest")}
	}
	return commandWizard, opts, parseOptions{}, nil
}

func runParse(opts parseOptions) error {
	path, err := filepath.Abs(opts.Path)
	if err != nil {
		return err
	}

	data, err := loadConfigData(path)
	if err != nil {
		return &exitError{Code: 1, Err: fmt.Errorf("failed to parse %s: %w", path, err)}
	}

	lines, err := renderParseLines(data)
	if err != nil {
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			return err
		}
		return &exitError{Code: 2, Err: err}
	}

	for _, line := range lines {
		fmt.Println(line)
	}
	return nil
}

func runWizard(opts wizardOptions) error {
	workspaceDir, err := filepath.Abs(opts.Directory)
	if err != nil {
		return err
	}

	info, err := os.Stat(workspaceDir)
	if err != nil || !info.IsDir() {
		return &exitError{Code: 1, Err: fmt.Errorf("workspace directory does not exist: %s", opts.Directory)}
	}

	manifestPath, err := filepath.Abs(opts.Manifest)
	if err != nil {
		return err
	}
	if stat, err := os.Stat(manifestPath); err != nil || stat.IsDir() {
		return &exitError{Code: 1, Err: fmt.Errorf("manifest file not found: %s", manifestPath)}
	}

	addons, err := parseAddonsManifest(manifestPath)
	if err != nil {
		return &exitError{Code: 1, Err: err}
	}

	repoRoot, err := resolveRepoRoot(workspaceDir, opts.RepoRoot)
	if err != nil {
		return &exitError{Code: 1, Err: err}
	}

	targetPath := filepath.Join(repoRoot, "sand.toml")
	data, err := loadConfigData(targetPath)
	if err != nil {
		return &exitError{Code: 1, Err: fmt.Errorf("failed to parse existing sand.toml: %w", err)}
	}

	warnings := validateExistingData(data)
	existingProfile := normalizeProfile(getStringValue(data, "profile"))
	if existingProfile == "" {
		existingProfile = "0"
	}

	existingMode := canonicalizeMode(getStringValue(data, "mode"))
	existingWSMode := getStringValue(data, "workspace_mode")
	if existingWSMode != "mount" && existingWSMode != "copy" {
		existingWSMode = "mount"
	}

	discoveredProfiles, profileWarning := discoverProfiles()

	model := newWizardModel(wizardConfig{
		RepoRoot:         repoRoot,
		TargetPath:       targetPath,
		ProfileWarning:   profileWarning,
		Discovered:       discoveredProfiles,
		Warnings:         warnings,
		Addons:           addons,
		ExistingProfile:  existingProfile,
		ExistingMode:     existingMode,
		ExistingWSMode:   existingWSMode,
		ExistingAddons:   getExistingAddons(data),
		ExistingVersions: getExistingVersions(data),
	})

	program := tea.NewProgram(model)
	finalModel, err := program.Run()
	if err != nil {
		return err
	}

	finished := finalModel.(wizardModel)
	if finished.cancelled {
		return &exitError{Code: 1, Err: fmt.Errorf("configuration cancelled")}
	}

	if !finished.writeChanges {
		fmt.Println("No changes written.")
		return nil
	}

	updateConfigData(
		data,
		finished.profile,
		finished.mode,
		finished.workspaceMode,
		finished.selectedAddonsOrdered(),
		finished.configureVersions,
		finished.versionUpdates,
	)

	if err := writeConfigData(targetPath, data); err != nil {
		return &exitError{Code: 1, Err: err}
	}

	fmt.Printf("Wrote %s\n", targetPath)
	fmt.Printf("Next: sand %s %s or just sand\n", finished.profile, finished.mode)
	return nil
}

func parseAddonsManifest(path string) ([]addon, error) {
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

	var addons []addon
	for _, row := range rows[1:] {
		name := fieldAt(row, header["name"])
		if name == "" {
			continue
		}

		modes := make(map[string]bool)
		for _, mode := range strings.Split(fieldAt(row, header["enabled_modes"]), ",") {
			mode = strings.TrimSpace(mode)
			if mode != "" {
				modes[mode] = true
			}
		}

		addons = append(addons, addon{
			Name:         name,
			Description:  fieldAt(row, header["description"]),
			EnabledModes: modes,
		})
	}
	return addons, nil
}

func fieldAt(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func resolveRepoRoot(directory string, override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}

	cmd := exec.Command("git", "-C", directory, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return directory, nil
	}

	root := strings.TrimSpace(string(output))
	if root == "" {
		return directory, nil
	}
	return filepath.Abs(root)
}

func discoverProfiles() ([]string, string) {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, "Docker was not found in PATH; profile discovery unavailable."
	}

	cmd := exec.Command("docker", "volume", "ls", "--format", "{{.Name}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Sprintf("Failed to list Docker volumes: %s", message)
	}

	profiles := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "agent-persist-") {
			continue
		}

		profile := normalizeProfile(strings.TrimPrefix(line, "agent-persist-"))
		if profile != "" {
			profiles[profile] = true
		}
	}

	discovered := make([]string, 0, len(profiles))
	for profile := range profiles {
		discovered = append(discovered, profile)
	}
	sort.Slice(discovered, func(i, j int) bool {
		return profileSortKey(discovered[i]) < profileSortKey(discovered[j])
	})
	return discovered, ""
}

func profileSortKey(profile string) string {
	if profile == "" {
		return "2:"
	}
	if profile[0] >= '0' && profile[0] <= '9' {
		return "0:" + profile
	}
	return "1:" + profile
}

func normalizeProfile(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if len(value) != 1 {
		return ""
	}
	char := value[0]
	if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'z') {
		return value
	}
	return ""
}

func canonicalizeMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "std", "standard", "":
		return "std"
	case "lax", "yolo", "strict":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "std"
	}
}

func loadConfigData(path string) (map[string]any, error) {
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

func writeConfigData(path string, data map[string]any) error {
	rendered, err := toml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to render sand.toml: %w", err)
	}
	if len(rendered) == 0 || rendered[len(rendered)-1] != '\n' {
		rendered = append(rendered, '\n')
	}
	return os.WriteFile(path, rendered, 0o644)
}

func renderParseLines(data map[string]any) ([]string, error) {
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

	for _, key := range scalarKeys {
		value, ok := data[key]
		if !ok || value == nil {
			continue
		}

		strValue, ok := value.(string)
		if !ok {
			return lines, &exitError{Code: 2, Err: fmt.Errorf("error\t%s\texpected string", key)}
		}
		lines = append(lines, fmt.Sprintf("scalar\t%s\t%s", key, strValue))
	}

	addonsValue, ok := data["addons"]
	if !ok || addonsValue == nil {
		return lines, nil
	}

	items, ok := addonsValue.([]any)
	if !ok {
		return lines, &exitError{Code: 2, Err: fmt.Errorf("error\taddons\texpected array of strings")}
	}
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			return lines, &exitError{Code: 2, Err: fmt.Errorf("error\taddons\texpected array of strings")}
		}
		lines = append(lines, fmt.Sprintf("addon\t%s\t", value))
	}
	return lines, nil
}

func getStringValue(data map[string]any, key string) string {
	value, ok := data[key]
	if !ok {
		return ""
	}
	strValue, ok := value.(string)
	if !ok {
		return ""
	}
	return strValue
}

func getExistingAddons(data map[string]any) map[string]bool {
	addonsValue, ok := data["addons"]
	if !ok {
		return map[string]bool{}
	}

	items, ok := addonsValue.([]any)
	if !ok {
		return map[string]bool{}
	}

	selected := map[string]bool{}
	for _, item := range items {
		value, ok := item.(string)
		if ok {
			selected[value] = true
		}
	}
	return selected
}

func getExistingVersions(data map[string]any) map[string]string {
	values := map[string]string{}
	for _, key := range versionKeys {
		if value := getStringValue(data, key); value != "" {
			values[key] = value
		}
	}
	return values
}

func validateExistingData(data map[string]any) []string {
	var warnings []string
	for _, key := range scalarKeys {
		value, ok := data[key]
		if !ok || value == nil {
			continue
		}
		if _, ok := value.(string); !ok {
			warnings = append(warnings, fmt.Sprintf("Existing sand.toml key %q is not a string; it will be kept until overwritten.", key))
		}
	}

	if addonsValue, ok := data["addons"]; ok && addonsValue != nil {
		if _, ok := addonsValue.([]any); !ok {
			warnings = append(warnings, "Existing addons key is not a list; the wizard will replace it.")
		}
	}
	return warnings
}

func updateConfigData(
	data map[string]any,
	profile string,
	mode string,
	workspaceMode string,
	addons []string,
	configureVersions bool,
	versionUpdates map[string]string,
) {
	data["profile"] = profile
	data["mode"] = mode
	data["workspace_mode"] = workspaceMode
	data["addons"] = addons

	if !configureVersions {
		return
	}

	for _, key := range versionKeys {
		value := strings.TrimSpace(versionUpdates[key])
		if value == "" {
			delete(data, key)
			continue
		}
		data[key] = value
	}
}

func newWizardModel(cfg wizardConfig) wizardModel {
	model := wizardModel{
		cfg:               cfg,
		step:              stepIntro,
		profile:           cfg.ExistingProfile,
		mode:              cfg.ExistingMode,
		workspaceMode:     cfg.ExistingWSMode,
		selectedAddons:    map[string]bool{},
		versionUpdates:    map[string]string{},
		configureVersions: false,
	}

	for idx, profile := range cfg.Discovered {
		if profile == cfg.ExistingProfile {
			model.cursor = idx
			break
		}
	}

	model.syncSelectedAddons()
	return model
}

func (m wizardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, nil
	case tea.KeyMsg:
		if key := msg.String(); key == "ctrl+c" {
			m.cancelled = true
			return m, tea.Quit
		}
	}

	switch m.step {
	case stepIntro:
		return m.updateIntro(msg)
	case stepProfileSelect:
		return m.updateProfileSelect(msg)
	case stepProfileCustom:
		return m.updateProfileCustom(msg)
	case stepModeSelect:
		return m.updateModeSelect(msg)
	case stepWorkspaceMode:
		return m.updateWorkspaceMode(msg)
	case stepAddons:
		return m.updateAddons(msg)
	case stepStrictAddons:
		return m.updateStrictAddons(msg)
	case stepVersionConfirm:
		return m.updateVersionConfirm(msg)
	case stepVersionInput:
		return m.updateVersionInput(msg)
	case stepReview:
		return m.updateReview(msg)
	default:
		return m, nil
	}
}

func (m wizardModel) View() string {
	switch m.step {
	case stepIntro:
		return m.viewIntro()
	case stepProfileSelect:
		return m.viewProfileSelect()
	case stepProfileCustom:
		return m.viewProfileCustom()
	case stepModeSelect:
		return m.viewModeSelect()
	case stepWorkspaceMode:
		return m.viewWorkspaceMode()
	case stepAddons:
		return m.viewAddons()
	case stepStrictAddons:
		return m.viewStrictAddons()
	case stepVersionConfirm:
		return m.viewVersionConfirm()
	case stepVersionInput:
		return m.viewVersionInput()
	case stepReview:
		return m.viewReview()
	default:
		return ""
	}
}

func (m wizardModel) updateIntro(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		m.step = stepProfileSelect
		m.cursor = m.defaultProfileCursor()
		return m, nil
	}
	return m, nil
}

func (m wizardModel) updateProfileSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	options := append([]string{}, m.cfg.Discovered...)
	options = append(options, "__custom__")

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			m.cursor = wrapCursor(m.cursor-1, len(options))
		case "down", "j":
			m.cursor = wrapCursor(m.cursor+1, len(options))
		case "enter":
			selected := options[m.cursor]
			if selected == "__custom__" {
				m.step = stepProfileCustom
				m.message = ""
				m.textInput = newTextInput(m.profile)
				return m, textinput.Blink
			}
			m.profile = selected
			m.step = stepModeSelect
			m.cursor = modeCursor(m.mode)
		}
	}
	return m, nil
}

func (m wizardModel) updateProfileCustom(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		profile := normalizeProfile(m.textInput.Value())
		if profile == "" {
			m.message = "Invalid profile. Use one character: 0-9 or a-z."
			return m, nil
		}
		m.profile = profile
		m.message = ""
		m.step = stepModeSelect
		m.cursor = modeCursor(m.mode)
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m wizardModel) updateModeSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			m.cursor = wrapCursor(m.cursor-1, len(modeOptions))
		case "down", "j":
			m.cursor = wrapCursor(m.cursor+1, len(modeOptions))
		case "enter":
			m.mode = modeOptions[m.cursor].Value
			if m.mode == "strict" {
				m.workspaceMode = "copy"
				m.syncSelectedAddons()
				m.step = stepStrictAddons
				return m, nil
			}
			m.step = stepWorkspaceMode
			m.cursor = workspaceModeCursor(m.workspaceMode)
		}
	}
	return m, nil
}

func (m wizardModel) updateWorkspaceMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			m.cursor = wrapCursor(m.cursor-1, len(workspaceModeOptions))
		case "down", "j":
			m.cursor = wrapCursor(m.cursor+1, len(workspaceModeOptions))
		case "enter":
			m.workspaceMode = workspaceModeOptions[m.cursor].Value
			m.syncSelectedAddons()
			m.step = stepAddons
			m.addonCursor = 0
		}
	}
	return m, nil
}

func (m wizardModel) updateAddons(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.cfg.Addons) == 0 {
		m.step = stepVersionConfirm
		m.cursor = 1
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			m.addonCursor = wrapCursor(m.addonCursor-1, len(m.cfg.Addons))
		case "down", "j":
			m.addonCursor = wrapCursor(m.addonCursor+1, len(m.cfg.Addons))
		case " ":
			current := m.cfg.Addons[m.addonCursor]
			if current.EnabledModes[m.mode] {
				m.selectedAddons[current.Name] = !m.selectedAddons[current.Name]
			}
		case "enter":
			m.step = stepVersionConfirm
			m.cursor = 1
		}
	}
	return m, nil
}

func (m wizardModel) updateStrictAddons(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		m.step = stepVersionConfirm
		m.cursor = 1
	}
	return m, nil
}

func (m wizardModel) updateVersionConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "left", "h", "up", "k":
			m.cursor = wrapCursor(m.cursor-1, 2)
		case "right", "l", "down", "j":
			m.cursor = wrapCursor(m.cursor+1, 2)
		case "enter":
			m.configureVersions = m.cursor == 0
			if m.configureVersions {
				m.versionUpdates = map[string]string{}
				m.versionIdx = 0
				m.message = ""
				m.textInput = newTextInput(m.cfg.ExistingVersions[versionKeys[m.versionIdx]])
				m.step = stepVersionInput
				return m, textinput.Blink
			}
			m.step = stepReview
			m.cursor = 0
		}
	}
	return m, nil
}

func (m wizardModel) updateVersionInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		m.versionUpdates[versionKeys[m.versionIdx]] = strings.TrimSpace(m.textInput.Value())
		m.versionIdx++
		if m.versionIdx >= len(versionKeys) {
			m.step = stepReview
			m.cursor = 0
			return m, nil
		}

		m.textInput = newTextInput(m.cfg.ExistingVersions[versionKeys[m.versionIdx]])
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m wizardModel) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "left", "h", "up", "k":
			m.cursor = wrapCursor(m.cursor-1, 2)
		case "right", "l", "down", "j":
			m.cursor = wrapCursor(m.cursor+1, 2)
		case "enter":
			m.writeChanges = m.cursor == 0
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m wizardModel) viewIntro() string {
	lines := []string{
		titleStyle.Render("sand config"),
		"",
		boxStyle.Render(strings.Join([]string{
			"This wizard creates or updates sand.toml at the repo root.",
			"Profiles map to persisted Docker volumes so auth and tool state can be isolated by profile.",
			fmt.Sprintf("Repo root: %s", m.cfg.RepoRoot),
			fmt.Sprintf("Target: %s", m.cfg.TargetPath),
		}, "\n")),
	}

	if m.cfg.ProfileWarning != "" {
		lines = append(lines, warning.Render(m.cfg.ProfileWarning))
	}

	if len(m.cfg.Discovered) > 0 {
		lines = append(lines, fmt.Sprintf("Discovered persisted profiles: %s", strings.Join(m.cfg.Discovered, ", ")))
	} else {
		lines = append(lines, muted.Render("No persisted profiles discovered."))
	}

	for _, message := range m.cfg.Warnings {
		lines = append(lines, warning.Render(message))
	}

	lines = append(lines, "", muted.Render("Press Enter to start. Ctrl+C cancels."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewProfileSelect() string {
	options := append([]string{}, m.cfg.Discovered...)
	options = append(options, "__custom__")

	lines := []string{
		titleStyle.Render("Select profile"),
		"Persisted auth, MCP config, and git state are scoped per profile.",
		"",
	}

	for idx, profile := range options {
		label := fmt.Sprintf("profile %s", profile)
		if profile == "__custom__" {
			label = "Custom profile"
		}
		lines = append(lines, renderCursorOption(idx == m.cursor, label))
	}

	lines = append(lines, "", muted.Render("Use ↑/↓, press Enter to continue."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewProfileCustom() string {
	lines := []string{
		titleStyle.Render("Custom profile"),
		"Enter a single character profile identifier: 0-9 or a-z.",
		"",
		m.textInput.View(),
	}
	if m.message != "" {
		lines = append(lines, errorStyle.Render(m.message))
	}
	lines = append(lines, "", muted.Render("Press Enter to continue."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewModeSelect() string {
	lines := []string{
		titleStyle.Render("Select security mode"),
		"",
	}
	for idx, opt := range modeOptions {
		lines = append(lines, renderCursorOption(idx == m.cursor, fmt.Sprintf("%-6s %s", opt.Value, opt.Description)))
	}
	lines = append(lines, "", muted.Render("Use ↑/↓, press Enter to continue."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewWorkspaceMode() string {
	lines := []string{
		titleStyle.Render("Select workspace mode"),
		"",
	}
	for idx, opt := range workspaceModeOptions {
		lines = append(lines, renderCursorOption(idx == m.cursor, fmt.Sprintf("%-6s %s", opt.Value, opt.Description)))
	}
	lines = append(lines, "", muted.Render("Use ↑/↓, press Enter to continue."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewAddons() string {
	lines := []string{
		titleStyle.Render("Select addons"),
		"Space toggles the highlighted addon.",
		"",
	}
	for idx, add := range m.cfg.Addons {
		prefix := "[ ]"
		if m.selectedAddons[add.Name] {
			prefix = "[x]"
		}
		label := fmt.Sprintf("%s %-16s %s", prefix, add.Name, add.Description)
		if !add.EnabledModes[m.mode] {
			label = fmt.Sprintf("%s [not available in %s]", label, m.mode)
			lines = append(lines, muted.Render(renderCursorOption(idx == m.addonCursor, label)))
			continue
		}
		lines = append(lines, renderCursorOption(idx == m.addonCursor, label))
	}
	lines = append(lines, "", muted.Render("Use ↑/↓, Space to toggle, Enter to continue."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewStrictAddons() string {
	lines := []string{
		titleStyle.Render("Addons disabled in strict mode"),
		warning.Render("strict mode enforces workspace_mode=copy and writes addons = []."),
		"",
	}
	if len(m.cfg.Addons) > 0 {
		for _, add := range m.cfg.Addons {
			configured := "no"
			if m.cfg.ExistingAddons[add.Name] {
				configured = "yes"
			}
			lines = append(lines, fmt.Sprintf("- %-16s configured=%s available=%s", add.Name, configured, enabledModesString(add)))
		}
	}
	lines = append(lines, "", muted.Render("Press Enter to continue."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewVersionConfirm() string {
	lines := []string{
		titleStyle.Render("Advanced runtime version pins"),
		"Configure advanced runtime version pins?",
		"",
		renderYesNo(m.cursor == 0, "Yes"),
		renderYesNo(m.cursor == 1, "No"),
		"",
	}
	if len(m.cfg.ExistingVersions) > 0 {
		keys := make([]string, 0, len(m.cfg.ExistingVersions))
		for key := range m.cfg.ExistingVersions {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		lines = append(lines, muted.Render(fmt.Sprintf("Existing advanced keys: %s", strings.Join(keys, ", "))))
	}
	lines = append(lines, muted.Render("Use ↑/↓ or ←/→, press Enter to continue."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewVersionInput() string {
	key := versionKeys[m.versionIdx]
	lines := []string{
		titleStyle.Render("Advanced runtime version pins"),
		fmt.Sprintf("%d/%d  %s", m.versionIdx+1, len(versionKeys), key),
		muted.Render("Leave blank to remove this key from sand.toml."),
		"",
		m.textInput.View(),
		"",
		muted.Render("Press Enter to continue."),
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewReview() string {
	lines := []string{
		titleStyle.Render("Review"),
		fmt.Sprintf("repo_root      %s", m.cfg.RepoRoot),
		fmt.Sprintf("target         %s", m.cfg.TargetPath),
		fmt.Sprintf("profile        %s", m.profile),
		fmt.Sprintf("mode           %s", m.mode),
		fmt.Sprintf("workspace_mode %s", m.workspaceMode),
		fmt.Sprintf("addons         %s", stringsJoinOrEmpty(m.selectedAddonsOrdered(), ", ", "(none)")),
	}

	if m.configureVersions {
		for _, key := range versionKeys {
			value := strings.TrimSpace(m.versionUpdates[key])
			if value == "" {
				lines = append(lines, fmt.Sprintf("%-14s (removed)", key))
				continue
			}
			lines = append(lines, fmt.Sprintf("%-14s %s", key, value))
		}
	} else {
		lines = append(lines, "version_pins   unchanged")
	}

	lines = append(lines, "", renderYesNo(m.cursor == 0, "Write changes"), renderYesNo(m.cursor == 1, "Cancel"), "")
	lines = append(lines, muted.Render("Use ↑/↓ or ←/→, press Enter to finish."))
	return strings.Join(lines, "\n") + "\n"
}

func (m *wizardModel) syncSelectedAddons() {
	m.selectedAddons = map[string]bool{}
	if m.mode == "strict" {
		return
	}
	for _, add := range m.cfg.Addons {
		if add.EnabledModes[m.mode] && m.cfg.ExistingAddons[add.Name] {
			m.selectedAddons[add.Name] = true
		}
	}
}

func (m wizardModel) selectedAddonsOrdered() []string {
	selectedAddons := []string{}
	for _, add := range m.cfg.Addons {
		if m.selectedAddons[add.Name] {
			selectedAddons = append(selectedAddons, add.Name)
		}
	}
	return selectedAddons
}

func (m wizardModel) defaultProfileCursor() int {
	for idx, profile := range m.cfg.Discovered {
		if profile == m.profile {
			return idx
		}
	}
	return len(m.cfg.Discovered)
}

func wrapCursor(next int, size int) int {
	if size == 0 {
		return 0
	}
	if next < 0 {
		return size - 1
	}
	if next >= size {
		return 0
	}
	return next
}

func modeCursor(mode string) int {
	for idx, opt := range modeOptions {
		if opt.Value == mode {
			return idx
		}
	}
	return 0
}

func workspaceModeCursor(mode string) int {
	for idx, opt := range workspaceModeOptions {
		if opt.Value == mode {
			return idx
		}
	}
	return 0
}

func enabledModesString(add addon) string {
	modes := make([]string, 0, len(add.EnabledModes))
	for mode := range add.EnabledModes {
		modes = append(modes, mode)
	}
	sort.Strings(modes)
	if len(modes) == 0 {
		return "-"
	}
	return strings.Join(modes, ",")
}

func renderCursorOption(current bool, label string) string {
	prefix := "  "
	if current {
		prefix = selected.Render("> ")
		return prefix + selected.Render(label)
	}
	return prefix + label
}

func renderYesNo(current bool, label string) string {
	if current {
		return renderCursorOption(true, label)
	}
	return renderCursorOption(false, label)
}

func newTextInput(value string) textinput.Model {
	input := textinput.New()
	input.Prompt = "> "
	input.SetValue(value)
	input.Focus()
	input.CursorEnd()
	return input
}

func stringsJoinOrEmpty(items []string, sep string, empty string) string {
	if len(items) == 0 {
		return empty
	}
	return strings.Join(items, sep)
}
