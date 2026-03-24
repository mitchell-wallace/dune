package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"claudebox/internal/dune/config"
	"claudebox/internal/dune/domain"
	"claudebox/internal/dune/gear"
	"claudebox/internal/dune/workspace"
)

var modeOptions = []option{
	{Value: string(domain.ModeStd), Description: "firewall enabled, curated gear available"},
	{Value: string(domain.ModeLax), Description: "firewall enabled, passwordless sudo"},
	{Value: string(domain.ModeYolo), Description: "firewall disabled, passwordless sudo"},
	{Value: string(domain.ModeStrict), Description: "firewall enabled, gear disabled, workspace copied (not mounted)"},
}

var workspaceModeOptions = []option{
	{Value: string(domain.WorkspaceModeMount), Description: "bind-mount workspace from host (read-write, default)"},
	{Value: string(domain.WorkspaceModeCopy), Description: "copy workspace into container (host filesystem unchanged, use git to sync)"},
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	boxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("6")).Padding(1, 2)
	selected   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	muted      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
)

type option struct {
	Value       string
	Description string
}

type wizardConfig struct {
	RepoRoot         string
	TargetPath       string
	ProfileWarning   string
	Discovered       []string
	Warnings         []string
	Gear             []domain.GearSpec
	ExistingProfile  string
	ExistingMode     string
	ExistingWSMode   string
	ExistingGear     map[string]bool
	ExistingVersions map[string]string
}

type wizardStep int

const (
	stepIntro wizardStep = iota
	stepProfileSelect
	stepProfileCustom
	stepModeSelect
	stepWorkspaceMode
	stepGear
	stepStrictGear
	stepVersionConfirm
	stepVersionInput
	stepReview
)

type wizardModel struct {
	cfg wizardConfig

	step wizardStep

	cursor     int
	gearCursor int
	versionIdx int

	textInput textinput.Model
	message   string

	profile           string
	mode              string
	workspaceMode     string
	selectedGear      map[string]bool
	configureVersions bool
	versionUpdates    map[string]string

	cancelled    bool
	writeChanges bool
}

func RunConfigWizard(directory, manifestPath string) error {
	workspaceDir, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	info, err := os.Stat(workspaceDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("workspace directory does not exist: %s", directory)
	}

	manifestPath, err = filepath.Abs(manifestPath)
	if err != nil {
		return err
	}

	specs, err := gear.ParseManifest(manifestPath)
	if err != nil {
		return err
	}

	repoRoot, err := workspace.ResolveRepoRoot(workspaceDir)
	if err != nil {
		return err
	}
	targetPath := filepath.Join(repoRoot, "dune.toml")
	data, err := config.Load(targetPath)
	if err != nil {
		return fmt.Errorf("failed to parse existing dune.toml: %w", err)
	}

	parsed, _, err := config.Parse(data)
	if err != nil && len(data) != 0 {
		return err
	}

	discoveredProfiles, profileWarning := discoverProfiles()
	model := newWizardModel(wizardConfig{
		RepoRoot:         repoRoot,
		TargetPath:       targetPath,
		ProfileWarning:   profileWarning,
		Discovered:       discoveredProfiles,
		Warnings:         config.ValidateExistingData(data),
		Gear:             specs,
		ExistingProfile:  defaultString(string(parsed.Profile), "0"),
		ExistingMode:     defaultString(string(parsed.Mode), "std"),
		ExistingWSMode:   defaultString(string(parsed.WorkspaceMode), "mount"),
		ExistingGear:     config.ExistingGear(data),
		ExistingVersions: config.ExistingVersions(data),
	})

	finalModel, err := tea.NewProgram(model).Run()
	if err != nil {
		return err
	}
	finished := finalModel.(wizardModel)
	if finished.cancelled {
		return fmt.Errorf("configuration cancelled")
	}
	if !finished.writeChanges {
		fmt.Println("No changes written.")
		return nil
	}

	finalConfig := parsed
	finalConfig.Profile = domain.Profile(finished.profile)
	finalConfig.Mode = domain.Mode(finished.mode)
	finalConfig.WorkspaceMode = domain.WorkspaceMode(finished.workspaceMode)
	finalConfig.Gear = make([]domain.GearName, 0, len(finished.selectedGearOrdered()))
	for _, gearName := range finished.selectedGearOrdered() {
		finalConfig.Gear = append(finalConfig.Gear, domain.GearName(gearName))
	}

	config.UpdateData(data, finalConfig, finished.configureVersions, finished.versionUpdates)
	if err := config.Write(targetPath, data); err != nil {
		return err
	}

	fmt.Printf("Wrote %s\n", targetPath)
	fmt.Printf("Next: dune %s %s or just dune\n", finished.profile, finished.mode)
	return nil
}

func discoverProfiles() ([]string, string) {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, "Docker was not found in PATH; profile discovery unavailable."
	}
	output, err := exec.Command("docker", "volume", "ls", "--format", "{{.Name}}").CombinedOutput()
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

		profile, ok := config.NormalizeProfile(strings.TrimPrefix(line, "agent-persist-"))
		if ok {
			profiles[string(profile)] = true
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

func newWizardModel(cfg wizardConfig) wizardModel {
	model := wizardModel{
		cfg:               cfg,
		step:              stepIntro,
		profile:           cfg.ExistingProfile,
		mode:              cfg.ExistingMode,
		workspaceMode:     cfg.ExistingWSMode,
		selectedGear:      map[string]bool{},
		versionUpdates:    map[string]string{},
		configureVersions: false,
	}
	model.syncSelectedGear()
	return model
}

func (m wizardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
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
	case stepGear:
		return m.updateGear(msg)
	case stepStrictGear:
		return m.updateStrictGear(msg)
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
	case stepGear:
		return m.viewGear()
	case stepStrictGear:
		return m.viewStrictGear()
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
		profile, ok := config.NormalizeProfile(m.textInput.Value())
		if !ok {
			m.message = "Invalid profile. Use one character: 0-9 or a-z."
			return m, nil
		}
		m.profile = string(profile)
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
				m.syncSelectedGear()
				m.step = stepStrictGear
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
			m.syncSelectedGear()
			m.step = stepGear
		}
	}
	return m, nil
}

func (m wizardModel) updateGear(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			m.gearCursor = wrapCursor(m.gearCursor-1, len(m.cfg.Gear))
		case "down", "j":
			m.gearCursor = wrapCursor(m.gearCursor+1, len(m.cfg.Gear))
		case " ":
			current := m.cfg.Gear[m.gearCursor]
			if current.EnabledModes[domain.Mode(m.mode)] {
				m.selectedGear[string(current.Name)] = !m.selectedGear[string(current.Name)]
			}
		case "enter":
			m.step = stepVersionConfirm
			m.cursor = 1
		}
	}
	return m, nil
}

func (m wizardModel) updateStrictGear(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				m.textInput = newTextInput(m.cfg.ExistingVersions[config.VersionKeys[m.versionIdx]])
				m.step = stepVersionInput
				return m, textinput.Blink
			}
			m.step = stepReview
		}
	}
	return m, nil
}

func (m wizardModel) updateVersionInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		m.versionUpdates[config.VersionKeys[m.versionIdx]] = strings.TrimSpace(m.textInput.Value())
		m.versionIdx++
		if m.versionIdx >= len(config.VersionKeys) {
			m.step = stepReview
			return m, nil
		}
		m.textInput = newTextInput(m.cfg.ExistingVersions[config.VersionKeys[m.versionIdx]])
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
		titleStyle.Render("dune config"),
		"",
		boxStyle.Render(strings.Join([]string{
			"This wizard creates or updates dune.toml at the repo root.",
			"Profiles map to persisted Docker volumes so auth and tool state can be isolated by profile.",
			fmt.Sprintf("Repo root: %s", m.cfg.RepoRoot),
			fmt.Sprintf("Target: %s", m.cfg.TargetPath),
		}, "\n")),
	}
	for _, warning := range m.cfg.Warnings {
		lines = append(lines, warnStyle.Render(warning))
	}
	lines = append(lines, "", muted.Render("Press Enter to start. Ctrl+C cancels."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewProfileSelect() string {
	options := append([]string{}, m.cfg.Discovered...)
	options = append(options, "__custom__")
	lines := []string{titleStyle.Render("Select profile"), ""}
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
	lines := []string{titleStyle.Render("Custom profile"), "", m.textInput.View()}
	if m.message != "" {
		lines = append(lines, errorStyle.Render(m.message))
	}
	lines = append(lines, "", muted.Render("Press Enter to continue."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewModeSelect() string {
	lines := []string{titleStyle.Render("Select security mode"), ""}
	for idx, opt := range modeOptions {
		lines = append(lines, renderCursorOption(idx == m.cursor, fmt.Sprintf("%-6s %s", opt.Value, opt.Description)))
	}
	lines = append(lines, "", muted.Render("Use ↑/↓, press Enter to continue."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewWorkspaceMode() string {
	lines := []string{titleStyle.Render("Select workspace mode"), ""}
	for idx, opt := range workspaceModeOptions {
		lines = append(lines, renderCursorOption(idx == m.cursor, fmt.Sprintf("%-6s %s", opt.Value, opt.Description)))
	}
	lines = append(lines, "", muted.Render("Use ↑/↓, press Enter to continue."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewGear() string {
	lines := []string{titleStyle.Render("Select gear"), ""}
	for idx, add := range m.cfg.Gear {
		prefix := "[ ]"
		if m.selectedGear[string(add.Name)] {
			prefix = "[x]"
		}
		label := fmt.Sprintf("%s %-16s %s", prefix, add.Name, add.Description)
		if !add.EnabledModes[domain.Mode(m.mode)] {
			lines = append(lines, muted.Render(renderCursorOption(idx == m.gearCursor, label+" [not available in "+m.mode+"]")))
			continue
		}
		lines = append(lines, renderCursorOption(idx == m.gearCursor, label))
	}
	lines = append(lines, "", muted.Render("Use ↑/↓, Space to toggle, Enter to continue."))
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewStrictGear() string {
	lines := []string{
		titleStyle.Render("Gear disabled in strict mode"),
		warnStyle.Render("strict mode enforces workspace_mode=copy and writes gear = []."),
		"",
		muted.Render("Press Enter to continue."),
	}
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
		muted.Render("Use ↑/↓ or ←/→, press Enter to continue."),
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m wizardModel) viewVersionInput() string {
	key := config.VersionKeys[m.versionIdx]
	lines := []string{
		titleStyle.Render("Advanced runtime version pins"),
		fmt.Sprintf("%d/%d  %s", m.versionIdx+1, len(config.VersionKeys), key),
		muted.Render("Leave blank to remove this key from dune.toml."),
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
		fmt.Sprintf("gear           %s", stringsJoinOrEmpty(m.selectedGearOrdered(), ", ", "(none)")),
	}
	if m.configureVersions {
		for _, key := range config.VersionKeys {
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
	lines = append(lines, "", renderYesNo(m.cursor == 0, "Write changes"), renderYesNo(m.cursor == 1, "Cancel"), "", muted.Render("Use ↑/↓ or ←/→, press Enter to finish."))
	return strings.Join(lines, "\n") + "\n"
}

func (m *wizardModel) syncSelectedGear() {
	m.selectedGear = map[string]bool{}
	if m.mode == "strict" {
		return
	}
	for _, add := range m.cfg.Gear {
		if add.EnabledModes[domain.Mode(m.mode)] && m.cfg.ExistingGear[string(add.Name)] {
			m.selectedGear[string(add.Name)] = true
		}
	}
}

func (m wizardModel) selectedGearOrdered() []string {
	selectedGear := []string{}
	for _, add := range m.cfg.Gear {
		if m.selectedGear[string(add.Name)] {
			selectedGear = append(selectedGear, string(add.Name))
		}
	}
	return selectedGear
}

func wrapCursor(next, size int) int {
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

func renderCursorOption(current bool, label string) string {
	if current {
		return selected.Render("> " + label)
	}
	return "  " + label
}

func renderYesNo(current bool, label string) string {
	return renderCursorOption(current, label)
}

func newTextInput(value string) textinput.Model {
	input := textinput.New()
	input.Prompt = "> "
	input.SetValue(value)
	input.Focus()
	input.CursorEnd()
	return input
}

func stringsJoinOrEmpty(items []string, sep, empty string) string {
	if len(items) == 0 {
		return empty
	}
	return strings.Join(items, sep)
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

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
