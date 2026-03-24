package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"claudebox/internal/orchestrator/contract"
	"claudebox/internal/orchestrator/progress"
	"claudebox/internal/orchestrator/runner"
	"claudebox/internal/orchestrator/state"
	orchtui "claudebox/internal/orchestrator/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) == 0 {
		return usage()
	}
	switch argv[0] {
	case "tui":
		return runTUI(argv[1:])
	case "run":
		return runBatch(argv[1:])
	case "progress":
		return runProgress(argv[1:])
	case "import-legacy":
		return runImportLegacy()
	case "version":
		fmt.Println(contract.BinaryName)
		return nil
	default:
		return usage()
	}
}

func usage() error {
	return errors.New("usage: sand-orch <tui|run|progress|import-legacy|version>")
}

func runTUI(argv []string) error {
	cfg := defaultConfig()
	for len(argv) > 0 {
		switch argv[0] {
		case "--iterations":
			if len(argv) < 2 {
				return errors.New("missing value for --iterations")
			}
			n, err := strconv.Atoi(argv[1])
			if err != nil {
				return fmt.Errorf("invalid iterations: %w", err)
			}
			cfg.Iterations = n
			argv = argv[2:]
		case "--agent":
			if len(argv) < 2 {
				return errors.New("missing value for --agent")
			}
			cfg.AgentSpecs = append(cfg.AgentSpecs, argv[1])
			argv = argv[2:]
		case "--auto-start":
			cfg.AutoStart = true
			argv = argv[1:]
		case "--exit-when-idle":
			cfg.ExitWhenIdle = true
			argv = argv[1:]
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
		return errors.New("usage: sand-orch progress <record|repair>")
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
	for len(argv) > 0 {
		switch argv[0] {
		case "--iterations":
			if len(argv) < 2 {
				return errors.New("missing value for --iterations")
			}
			n, err := strconv.Atoi(argv[1])
			if err != nil {
				return fmt.Errorf("invalid iterations: %w", err)
			}
			cfg.Iterations = n
			argv = argv[2:]
		case "--agent":
			if len(argv) < 2 {
				return errors.New("missing value for --agent")
			}
			cfg.AgentSpecs = append(cfg.AgentSpecs, argv[1])
			argv = argv[2:]
		default:
			return fmt.Errorf("unknown run arg: %s", argv[0])
		}
	}
	if cfg.Iterations == 0 {
		cfg.Iterations = 1
	}
	r := runner.New(runner.Config{
		WorkspaceDir:     cfg.WorkspaceDir,
		DataDir:          cfg.DataDir,
		RepoProgressPath: cfg.RepoProgressPath,
		Iterations:       cfg.Iterations,
		AgentSpecs:       cfg.AgentSpecs,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
	})
	if err := r.EnsureInitialized(); err != nil {
		return err
	}
	_, err := r.Run(context.Background())
	return err
}

func runProgressRecord() error {
	dataDir := getenvOr(contract.EnvDataDir, contract.ContainerDataRoot)
	repoPath := getenvOr(contract.EnvRepoProgressPath, contract.RepoProgressPath("/workspace"))
	sessionRaw := os.Getenv(contract.EnvSessionID)
	if sessionRaw == "" {
		return errors.New("SAND_ORCH_SESSION_ID is required for progress record")
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
	return orchtui.Config{
		WorkspaceDir:     workspaceDir,
		DataDir:          dataDir,
		RepoProgressPath: repoPath,
		Iterations:       1,
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
