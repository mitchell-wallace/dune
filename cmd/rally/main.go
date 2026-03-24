package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	contract "claudebox/internal/contracts/rally"
	duneconfig "claudebox/internal/dune/config"
	duneworkspace "claudebox/internal/dune/workspace"
	"claudebox/internal/rally/progress"
	"claudebox/internal/rally/runner"
	"claudebox/internal/rally/state"
	orchtui "claudebox/internal/rally/tui"
	"claudebox/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) == 0 {
		printUsage()
		return nil
	}
	switch argv[0] {
	case "tui":
		return runTUI(argv[1:])
	case "run":
		return runBatch(argv[1:])
	case "progress":
		return runProgress(argv[1:])
	case "instructions":
		return runInstructions(argv[1:])
	case "init":
		return runInit()
	case "import-legacy":
		return runImportLegacy()
	case "version":
		fmt.Printf("%s %s\n", contract.BinaryName, version.String())
		return nil
	case "--help", "-h", "help":
		printUsage()
		return nil
	default:
		printUsage()
		return nil
	}
}

func printUsage() {
	fmt.Print(`rally - agent orchestrator

Commands:
  run [prompt...]          Run a batch of agent sessions
  tui                      Interactive terminal UI
  init                     Interactive setup wizard
  instructions <cmd>       Manage project instructions (edit, show)
  progress <cmd>           Session progress (record, repair)
  version                  Print version

Flags for run/tui:
  --iterations N           Number of iterations (default: 1, scout: 5)
  --agent SPEC             Agent mix (repeatable, e.g. cc:2 cx:1)
  --beads [auto|true|false] Beads task source (default: from env/config)
  --scout [focus]          Scout mode: explore, don't change code

Examples:
  rally run don't touch auth, just fix tests
  rally run --beads --iterations 3
  rally run --scout "error handling"
  rally tui --auto-start --beads auto
`)
}

func runTUI(argv []string) error {
	cfg := defaultConfig()
	for len(argv) > 0 {
		switch argv[0] {
		case "--iterations":
			if len(argv) < 2 {
				return fmt.Errorf("missing value for --iterations")
			}
			n, err := strconv.Atoi(argv[1])
			if err != nil {
				return fmt.Errorf("invalid iterations: %w", err)
			}
			cfg.Iterations = n
			argv = argv[2:]
		case "--agent":
			if len(argv) < 2 {
				return fmt.Errorf("missing value for --agent")
			}
			cfg.AgentSpecs = append(cfg.AgentSpecs, argv[1])
			argv = argv[2:]
		case "--auto-start":
			cfg.AutoStart = true
			argv = argv[1:]
		case "--exit-when-idle":
			cfg.ExitWhenIdle = true
			argv = argv[1:]
		case "--beads":
			argv = argv[1:]
			cfg.BeadsMode = "true"
			if len(argv) > 0 && !strings.HasPrefix(argv[0], "--") {
				switch argv[0] {
				case "auto", "true", "false":
					cfg.BeadsMode = argv[0]
					argv = argv[1:]
				}
			}
		default:
			return fmt.Errorf("unknown tui arg: %s", argv[0])
		}
	}
	if cfg.Iterations == 0 {
		cfg.Iterations = 1
	}
	return orchtui.Run(cfg)
}

func runProgress(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("usage: rally progress <record|repair>")
	}
	switch argv[0] {
	case "record":
		return runProgressRecord()
	case "repair":
		return runProgressRepair()
	default:
		return fmt.Errorf("unknown progress command: %s", argv[0])
	}
}

func runBatch(argv []string) error {
	cfg := defaultConfig()
	var remaining []string
	beadsMode := os.Getenv(contract.EnvBeads)
	scoutMode := false
	scoutFocus := ""
	iterationsExplicit := false
	for len(argv) > 0 {
		switch argv[0] {
		case "--iterations":
			if len(argv) < 2 {
				return fmt.Errorf("missing value for --iterations")
			}
			n, err := strconv.Atoi(argv[1])
			if err != nil {
				return fmt.Errorf("invalid iterations: %w", err)
			}
			cfg.Iterations = n
			iterationsExplicit = true
			argv = argv[2:]
		case "--agent":
			if len(argv) < 2 {
				return fmt.Errorf("missing value for --agent")
			}
			cfg.AgentSpecs = append(cfg.AgentSpecs, argv[1])
			argv = argv[2:]
		case "--beads":
			argv = argv[1:]
			beadsMode = "true"
			if len(argv) > 0 && !strings.HasPrefix(argv[0], "--") {
				switch argv[0] {
				case "auto", "true", "false":
					beadsMode = argv[0]
					argv = argv[1:]
				}
			}
		case "--scout":
			argv = argv[1:]
			scoutMode = true
			if len(argv) > 0 && !strings.HasPrefix(argv[0], "--") {
				scoutFocus = argv[0]
				argv = argv[1:]
			}
		default:
			remaining = append(remaining, argv[0])
			argv = argv[1:]
		}
	}

	inlinePrompt := strings.Join(remaining, " ")

	iterations := cfg.Iterations
	if iterations == 0 {
		iterations = 1
	}
	if scoutMode && !iterationsExplicit {
		iterations = 5
	}

	r := runner.New(runner.Config{
		WorkspaceDir:     cfg.WorkspaceDir,
		DataDir:          cfg.DataDir,
		RepoProgressPath: cfg.RepoProgressPath,
		Iterations:       iterations,
		AgentSpecs:       cfg.AgentSpecs,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		BeadsMode:        beadsMode,
		InlinePrompt:     inlinePrompt,
		ScoutMode:        scoutMode,
		ScoutFocus:       scoutFocus,
		ClaudeModel:      cfg.ClaudeModel,
		CodexModel:       cfg.CodexModel,
		GeminiModel:      cfg.GeminiModel,
		OpenCodeModel:    cfg.OpenCodeModel,
	})
	if err := r.EnsureInitialized(); err != nil {
		return err
	}
	_, err := r.Run(context.Background())
	return err
}

func runInstructions(argv []string) error {
	if len(argv) == 0 {
		fmt.Println("usage: rally instructions <edit|show>")
		return nil
	}
	cfg := defaultConfig()
	path := filepath.Join(cfg.DataDir, "instructions.md")

	switch argv[0] {
	case "edit":
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			header := "# Rally Project Instructions\n\n# Add persistent instructions for rally agents below.\n# These are included in every agent session prompt.\n"
			if err := os.WriteFile(path, []byte(header), 0o644); err != nil {
				return err
			}
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		cmd := exec.Command(editor, path)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	case "show":
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("(no project instructions set)")
				return nil
			}
			return err
		}
		fmt.Print(string(data))
		return nil
	default:
		return fmt.Errorf("unknown instructions command: %s", argv[0])
	}
}

func runInit() error {
	cfg := defaultConfig()
	scanner := bufio.NewScanner(os.Stdin)
	instructionsPath := filepath.Join(cfg.DataDir, "instructions.md")
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return err
	}

	var instructions []string

	// Question 1: Beads
	fmt.Print("Are you using beads for task tracking? [y/n/auto] ")
	beadsAnswer := "auto"
	if scanner.Scan() {
		switch strings.TrimSpace(strings.ToLower(scanner.Text())) {
		case "y", "yes", "true":
			beadsAnswer = "true"
		case "n", "no", "false":
			beadsAnswer = "false"
		default:
			beadsAnswer = "auto"
		}
	}
	fmt.Printf("  beads = %s\n", beadsAnswer)

	// Write beads setting to dune.toml if in a workspace
	duneTomlPath := filepath.Join(cfg.WorkspaceDir, "dune.toml")
	writeDuneToml(duneTomlPath, "beads", beadsAnswer)

	// Question 2: Task source (if not beads)
	if beadsAnswer == "false" {
		fmt.Println("\nWhere should agents look for plans/specs?")
		fmt.Println("  1. Files (provide path)")
		fmt.Println("  2. MCP tool (describe)")
		fmt.Println("  3. CLI command (describe)")
		fmt.Println("  4. N/A")
		fmt.Print("Choice [1-4]: ")
		if scanner.Scan() {
			choice := strings.TrimSpace(scanner.Text())
			switch choice {
			case "1":
				fmt.Print("Path to plans/specs: ")
				if scanner.Scan() {
					path := strings.TrimSpace(scanner.Text())
					if path != "" {
						instructions = append(instructions, fmt.Sprintf("## Task Source\nLook for plans and specs in: %s", path))
					}
				}
			case "2":
				fmt.Print("Describe the MCP tool to use: ")
				if scanner.Scan() {
					desc := strings.TrimSpace(scanner.Text())
					if desc != "" {
						instructions = append(instructions, fmt.Sprintf("## Task Source\nUse the following MCP tool to find work: %s", desc))
					}
				}
			case "3":
				fmt.Print("CLI command to find tasks: ")
				if scanner.Scan() {
					cmd := strings.TrimSpace(scanner.Text())
					if cmd != "" {
						instructions = append(instructions, fmt.Sprintf("## Task Source\nRun this command to find available tasks: %s", cmd))
					}
				}
			}
		}
	}

	// Question 3: Priorities
	fmt.Print("\nWhat are your priorities for rally agents in review/scout mode?\n(free text, or press enter to skip): ")
	if scanner.Scan() {
		priorities := strings.TrimSpace(scanner.Text())
		if priorities != "" {
			instructions = append(instructions, fmt.Sprintf("## Agent Priorities\n%s", priorities))
		}
	}

	// Write instructions file
	if len(instructions) > 0 {
		content := "# Rally Project Instructions\n\n" + strings.Join(instructions, "\n\n") + "\n"
		if err := os.WriteFile(instructionsPath, []byte(content), 0o644); err != nil {
			return err
		}
		fmt.Printf("\nWrote instructions to %s\n", instructionsPath)
	}

	// Question 4: Scout
	fmt.Print("\nRun a scout session to prepare tasks for future sessions? [y/n] ")
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "y" || answer == "yes" {
			fmt.Println("Starting scout session (5 iterations)...")
			return runBatch([]string{"--scout", "--beads", beadsAnswer})
		}
	}

	fmt.Println("\nDone! Run `rally run` to start an agent session.")
	return nil
}

// writeDuneToml writes a single key to dune.toml, creating it if needed.
// Intentionally simple: appends or updates the key.
func writeDuneToml(path, key, value string) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), key+" ") || strings.HasPrefix(strings.TrimSpace(line), key+"=") {
			lines[i] = fmt.Sprintf("%s = %q", key, value)
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, fmt.Sprintf("%s = %q", key, value))
	}
	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	_ = os.WriteFile(path, []byte(content), 0o644)
}

func runProgressRecord() error {
	dataDir := getenvOr(contract.EnvDataDir, contract.ContainerDataRoot)
	repoPath := getenvOr(contract.EnvRepoProgressPath, contract.RepoProgressPath("/workspace"))
	sessionRaw := os.Getenv(contract.EnvSessionID)
	if sessionRaw == "" {
		return fmt.Errorf("%s is required for progress record", contract.EnvSessionID)
	}
	sessionID, err := strconv.Atoi(sessionRaw)
	if err != nil {
		return err
	}
	input, err := progress.ParseRecordInput(os.Stdin)
	if err != nil {
		return err
	}
	if err := progress.UpdateSessionMeta(dataDir, sessionID, func(meta *progress.SessionMeta) error {
		progress.ApplyRecord(meta, input)
		return nil
	}); err != nil {
		return err
	}
	st, err := state.NewStore(dataDir).Load()
	if err != nil {
		return err
	}
	_, err = progress.RebuildRepoProgress(dataDir, repoPath, activeBatchMap(st.ActiveBatch))
	return err
}

func runProgressRepair() error {
	cfg := defaultConfig()
	st, err := state.NewStore(cfg.DataDir).Load()
	if err != nil {
		return err
	}
	_, err = progress.RebuildRepoProgress(cfg.DataDir, cfg.RepoProgressPath, activeBatchMap(st.ActiveBatch))
	return err
}

func runImportLegacy() error {
	cfg := defaultConfig()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return err
	}
	st := state.NewStore(cfg.DataDir)
	current, err := st.Load()
	if err != nil {
		return err
	}
	return st.Save(current)
}

func defaultConfig() orchtui.Config {
	containerName := getenvOr(contract.EnvContainerName, "local")
	env := contract.ContainerEnv(containerName)
	dataDir := getenvOr(contract.EnvDataDir, env[contract.EnvDataDir])
	repoPath := getenvOr(contract.EnvRepoProgressPath, env[contract.EnvRepoProgressPath])
	workspaceDir := getenvOr(contract.EnvWorkspaceDir, "/workspace")
	beadsMode := os.Getenv(contract.EnvBeads)
	modelDefaults := loadModelDefaults(workspaceDir)
	return orchtui.Config{
		WorkspaceDir:     workspaceDir,
		DataDir:          dataDir,
		RepoProgressPath: repoPath,
		Iterations:       1,
		BeadsMode:        beadsMode,
		ClaudeModel:      modelDefaults.ClaudeModel,
		CodexModel:       modelDefaults.CodexModel,
		GeminiModel:      modelDefaults.GeminiModel,
		OpenCodeModel:    modelDefaults.OpenCodeModel,
	}
}

func getenvOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func activeBatchMap(batch *state.BatchState) map[string]any {
	if batch == nil {
		return nil
	}
	return map[string]any{
		"batch_id":             batch.BatchID,
		"target_iterations":    batch.TargetIterations,
		"completed_iterations": batch.CompletedIterations,
		"agent_mix":            batch.AgentMix,
		"started_at":           batch.StartedAt,
		"ended_at":             batch.EndedAt,
	}
}

func loadModelDefaults(workspaceDir string) runner.Config {
	configPath, err := duneworkspace.FindDuneToml(workspaceDir)
	if err != nil || configPath == "" {
		return runner.Config{}
	}

	data, err := duneconfig.Load(configPath)
	if err != nil {
		return runner.Config{}
	}
	parsed, _, err := duneconfig.Parse(data)
	if err != nil {
		return runner.Config{}
	}

	return runner.Config{
		ClaudeModel:   parsed.ClaudeModel,
		CodexModel:    parsed.CodexModel,
		GeminiModel:   parsed.GeminiModel,
		OpenCodeModel: parsed.OpenCodeModel,
	}
}
