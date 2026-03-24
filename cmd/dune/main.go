package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"claudebox/internal/contracts/rally"
	"claudebox/internal/dune/cli"
	"claudebox/internal/dune/config"
	"claudebox/internal/dune/container"
	"claudebox/internal/dune/devcontainer"
	"claudebox/internal/dune/domain"
	"claudebox/internal/dune/gear"
	"claudebox/internal/dune/tasks"
	"claudebox/internal/dune/tui"
	"claudebox/internal/dune/workspace"
)

type repoPaths struct {
	Root         string
	Devcontainer string
	Manifest     string
}

type gearContainer interface {
	ContainerFileExists(ctx context.Context, name, path string) bool
	ExecInContainer(ctx context.Context, name string, env map[string]string, args ...string) error
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	opts, err := cli.Parse(argv)
	if err != nil {
		return err
	}

	paths, err := locateRepoPaths()
	if err != nil {
		return err
	}

	_ = tasks.Event{Type: tasks.EventConfigResolved, Timestamp: time.Now()}

	switch opts.Command {
	case cli.CommandConfig:
		return tui.RunConfigWizard(defaultWorkspaceInput(opts.WorkspaceInput), paths.Manifest)
	case cli.CommandRebuild:
		return runRebuild(context.Background(), opts, paths)
	case cli.CommandRallyBuild:
		return runRallyBuild(context.Background(), opts, paths)
	default:
		return runDune(context.Background(), opts, paths)
	}
}

func runRebuild(ctx context.Context, opts cli.Options, paths repoPaths) error {
	ref, err := workspace.Resolve(defaultWorkspaceInput(opts.WorkspaceInput))
	if err != nil {
		return err
	}
	cfg, _, err := resolveConfig(ref)
	if err != nil {
		return err
	}

	identity := workspace.ContainerIdentity(ref, cfg.Profile)
	docker := container.NewClient(container.OSRunner{})
	if docker.ContainerExists(ctx, identity.Name) {
		fmt.Printf("Tearing down container: %s\n", identity.Name)
		if err := docker.RemoveContainer(ctx, identity.Name); err != nil {
			return err
		}
	} else {
		fmt.Printf("No existing container found: %s (will build fresh)\n", identity.Name)
	}

	return runDune(ctx, cli.Options{Command: cli.CommandRun, WorkspaceInput: ref.Dir}, paths)
}

func runRallyBuild(ctx context.Context, opts cli.Options, paths repoPaths) error {
	// Build the binary on the host.
	fmt.Println("Building rally binary...")
	if err := buildRallyBinary(ctx, paths.Root); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}
	ver := readVersion(paths.Root)
	commit := gitCommitShort(ctx, paths.Root)
	fmt.Printf("Built rally %s (%s)\n", ver, commit)

	// Resolve the target container.
	ref, err := workspace.Resolve(defaultWorkspaceInput(opts.WorkspaceInput))
	if err != nil {
		return err
	}
	cfg, _, err := resolveConfig(ref)
	if err != nil {
		return err
	}
	identity := workspace.ContainerIdentity(ref, cfg.Profile)

	docker := container.NewClient(container.OSRunner{})
	if !docker.ContainerExists(ctx, identity.Name) {
		return fmt.Errorf("container %s does not exist", identity.Name)
	}
	if !docker.ContainerRunning(ctx, identity.Name) {
		fmt.Printf("Starting container %s...\n", identity.Name)
		if err := docker.StartContainer(ctx, identity.Name); err != nil {
			return err
		}
	}

	// Ensure the persistent binary directory exists inside the container.
	if err := docker.ExecInContainer(ctx, identity.Name, nil, "mkdir", "-p", contract.PersistentBinaryDir); err != nil {
		return fmt.Errorf("failed to create %s in container: %w", contract.PersistentBinaryDir, err)
	}

	// Copy the built binary into the container's persistent volume.
	hostBinary := contract.HostBinaryPath(paths.Root)
	containerDest := contract.PersistentBinaryDir + "/" + contract.BinaryName
	runner := container.OSRunner{}
	if err := runner.Run(ctx, "docker", "cp", hostBinary, identity.Name+":"+containerDest); err != nil {
		return fmt.Errorf("docker cp failed: %w", err)
	}

	fmt.Printf("Pushed rally %s (%s) to %s\n", ver, commit, identity.Name)
	return nil
}

func runDune(ctx context.Context, opts cli.Options, paths repoPaths) error {
	ref, err := workspace.Resolve(defaultWorkspaceInput(opts.WorkspaceInput))
	if err != nil {
		return err
	}

	fileConfig, warnings, err := resolveConfig(ref)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "WARNING: %s\n", warning)
	}

	cfg := fileConfig
	if opts.ProfileExplicit {
		cfg.Profile = opts.Profile
	}
	if opts.ModeExplicit {
		cfg.Mode = opts.Mode
	}
	if cfg.Mode == "" {
		cfg.Mode = domain.ModeStd
	}
	if cfg.Profile == "" {
		cfg.Profile = domain.Profile("0")
	}
	if cfg.WorkspaceMode == "" {
		cfg.WorkspaceMode = domain.WorkspaceModeMount
	}
	if cfg.Mode == domain.ModeStrict {
		if cfg.WorkspaceMode == domain.WorkspaceModeMount {
			fmt.Fprintln(os.Stderr, "WARNING: strict mode enforces workspace_mode=copy; overriding configured 'mount'.")
		}
		cfg.WorkspaceMode = domain.WorkspaceModeCopy
	}

	identity := workspace.ContainerIdentity(ref, cfg.Profile)
	docker := container.NewClient(container.OSRunner{})

	buildGearArg := gear.BuildCSV(cfg.Gear, func(message string) {
		fmt.Fprintf(os.Stderr, "WARNING: %s\n", message)
	})

	if docker.ContainerExists(ctx, identity.Name) {
		existingMode, err := docker.ResolveContainerMode(ctx, identity.Name)
		if err != nil {
			return err
		}
		if cfg.Mode != existingMode {
			fmt.Fprintf(os.Stderr, "WARNING: Container '%s' already exists with security mode '%s', but you requested '%s'. The existing container mode is immutable and will be used.\n", identity.Name, existingMode, cfg.Mode)
			cfg.Mode = existingMode
		}

		existingWorkspaceMode, err := docker.ResolveWorkspaceMode(ctx, identity.Name)
		if err != nil {
			return err
		}
		if cfg.WorkspaceMode != existingWorkspaceMode {
			fmt.Fprintf(os.Stderr, "WARNING: Container '%s' already exists with workspace_mode '%s', but you requested '%s'. The existing workspace_mode is immutable and will be used.\n", identity.Name, existingWorkspaceMode, cfg.WorkspaceMode)
			cfg.WorkspaceMode = existingWorkspaceMode
		}
	} else {
		fmt.Printf("Provisioning dev container via devcontainers CLI (profile=%s mode=%s workspace_mode=%s)\n", cfg.Profile, cfg.Mode, cfg.WorkspaceMode)
		if buildGearArg != "" {
			fmt.Printf("Build-time gear requested from dune.toml: %s\n", buildGearArg)
		}

		effectiveConfig, cleanup, err := devcontainer.PrepareConfig(paths.Devcontainer, cfg.WorkspaceMode)
		if err != nil {
			return err
		}
		defer cleanup()

		if err := buildRallyBinary(ctx, paths.Root); err != nil {
			return err
		}
		if err := injectRallyMount(effectiveConfig, paths.Root, identity.Name, cfg.Beads); err != nil {
			return err
		}

		cmd := exec.CommandContext(ctx, "npx", "@devcontainers/cli", "up",
			"--workspace-folder", ref.Dir,
			"--config", effectiveConfig,
			"--id-label", "devcontainer.local_folder="+ref.Dir,
			"--id-label", "devcontainer.config_file="+paths.Devcontainer,
			"--id-label", "dune.profile="+string(cfg.Profile),
		)
		cmd.Dir = paths.Root
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(),
			"DUNE_PROFILE="+string(cfg.Profile),
			"DUNE_SECURITY_MODE="+string(cfg.Mode),
			"DUNE_WORKSPACE_MODE="+string(cfg.WorkspaceMode),
			"DUNE_BUILD_MODE="+string(cfg.Mode),
			"DUNE_BUILD_GEAR="+buildGearArg,
			"DUNE_PYTHON_VERSION="+cfg.PythonVersion,
			"DUNE_UV_VERSION="+cfg.UVVersion,
			"DUNE_GO_VERSION="+cfg.GoVersion,
			"DUNE_RUST_VERSION="+cfg.RustVersion,
		)
		if err := cmd.Run(); err != nil {
			return err
		}

		createdID, err := docker.FindCreatedContainerID(ctx, ref.Dir, paths.Devcontainer, string(cfg.Profile))
		if err != nil {
			return err
		}
		if createdID == "" {
			return fmt.Errorf("could not find container created by devcontainers CLI")
		}
		createdName, err := docker.ContainerName(ctx, createdID)
		if err != nil {
			return err
		}
		if createdName != identity.Name {
			if err := docker.RenameContainer(ctx, createdID, identity.Name); err != nil {
				return err
			}
		}

		if cfg.WorkspaceMode == domain.WorkspaceModeCopy {
			fmt.Println("Copying workspace into container (workspace_mode=copy)...")
			if err := docker.CopyWorkspace(ctx, ref.Dir, identity.Name); err != nil {
				return err
			}
			fmt.Println("Workspace copied. Host filesystem will not be modified.")
		}
	}

	if !docker.ContainerRunning(ctx, identity.Name) {
		if err := docker.StartContainer(ctx, identity.Name); err != nil {
			return err
		}
	}

	if value, err := docker.ContainerEnvValue(ctx, identity.Name, contract.EnvDataDir); err == nil && value == "" {
		fmt.Fprintf(os.Stderr, "WARNING: Container '%s' is missing rally wiring. Recreate it once with `dune rebuild` to mount the runtime binary.\n", identity.Name)
	}

	if err := applyConfiguredGear(ctx, docker, cfg, identity.Name, paths.Manifest); err != nil {
		return err
	}

	return docker.AttachShell(ctx, identity.Name)
}

func buildRallyBinary(ctx context.Context, repoRoot string) error {
	binPath := contract.HostBinaryPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		return err
	}

	ver := readVersion(repoRoot)
	commit := gitCommitShort(ctx, repoRoot)

	cmdArgs := contract.HostBinaryBuildCommandWithVersion(repoRoot, ver, commit)
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	return cmd.Run()
}

func readVersion(repoRoot string) string {
	data, err := os.ReadFile(filepath.Join(repoRoot, "VERSION"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func gitCommitShort(ctx context.Context, repoRoot string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func injectRallyMount(configPath, repoRoot, containerName, beadsMode string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	mounts := asStringSlice(cfg["mounts"])
	hostBinary := contract.HostBinaryPath(repoRoot)
	mounts = append(mounts, fmt.Sprintf("source=%s,target=%s,type=bind,readonly", hostBinary, contract.ContainerBinaryPath))
	cfg["mounts"] = mounts

	envMap := asMapAny(cfg["containerEnv"])
	for key, value := range contract.ContainerEnv(containerName) {
		envMap[key] = value
	}
	if beadsMode != "" {
		envMap[contract.EnvBeads] = beadsMode
	}
	cfg["containerEnv"] = envMap

	rendered, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, append(rendered, '\n'), 0o644)
}

func asStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]string); ok {
			return append([]string{}, typed...)
		}
		return []string{}
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}
	return result
}

func asMapAny(value any) map[string]any {
	if result, ok := value.(map[string]any); ok && result != nil {
		return result
	}
	return map[string]any{}
}

func resolveConfig(ref domain.WorkspaceRef) (domain.DuneConfig, []string, error) {
	if ref.ConfigPath == "" {
		return config.DefaultConfig(), nil, nil
	}
	data, err := config.Load(ref.ConfigPath)
	if err != nil {
		return domain.DuneConfig{}, nil, err
	}
	cfg, warnings, err := config.Parse(data)
	if err != nil {
		return domain.DuneConfig{}, nil, err
	}
	fmt.Printf("Using dune.toml config: %s\n", ref.ConfigPath)
	return cfg, warnings, nil
}

func applyConfiguredGear(ctx context.Context, docker gearContainer, cfg domain.DuneConfig, containerName, manifestPath string) error {
	if len(cfg.Gear) == 0 {
		return nil
	}
	if cfg.Mode == domain.ModeStrict {
		fmt.Fprintln(os.Stderr, "WARNING: dune.toml lists gear but mode is strict; ignoring configured gear.")
		return nil
	}

	specs, err := gear.ParseManifest(manifestPath)
	if err != nil {
		return err
	}
	known := gear.IndexByName(specs)
	requested := gear.DedupeRequested(cfg.Gear)
	fmt.Printf("Applying configured gear from dune.toml (%d requested)...\n", len(requested))

	env := gearEnv(cfg)

	installed := 0
	skippedInstalled := 0
	skippedUnknown := 0
	skippedInvalid := 0
	for _, item := range requested {
		name := string(item)
		if !gear.IsValidName(name) {
			fmt.Fprintf(os.Stderr, "WARNING: Invalid gear name in dune.toml skipped: %s\n", name)
			skippedInvalid++
			continue
		}
		if _, ok := known[name]; !ok {
			fmt.Fprintf(os.Stderr, "WARNING: Unknown gear entry in dune.toml skipped: %s\n", name)
			skippedUnknown++
			continue
		}
		if docker.ContainerFileExists(ctx, containerName, "/persist/agent/gear/"+name+".installed") {
			skippedInstalled++
			continue
		}
		fmt.Printf("Installing gear from dune.toml: %s\n", name)
		if err := docker.ExecInContainer(ctx, containerName, env, "gear", "install", name); err != nil {
			return fmt.Errorf("failed to install configured gear %q: %w", name, err)
		}
		installed++
	}

	fmt.Printf("dune.toml gear summary: installed=%d skipped_installed=%d skipped_unknown=%d skipped_invalid=%d\n", installed, skippedInstalled, skippedUnknown, skippedInvalid)
	return nil
}

func gearEnv(cfg domain.DuneConfig) map[string]string {
	env := map[string]string{}
	if cfg.PythonVersion != "" {
		env["DUNE_PYTHON_VERSION"] = cfg.PythonVersion
	}
	if cfg.UVVersion != "" {
		env["DUNE_UV_VERSION"] = cfg.UVVersion
	}
	if cfg.GoVersion != "" {
		env["DUNE_GO_VERSION"] = cfg.GoVersion
	}
	if cfg.RustVersion != "" {
		env["DUNE_RUST_VERSION"] = cfg.RustVersion
	}
	return env
}

func locateRepoPaths() (repoPaths, error) {
	root := strings.TrimSpace(os.Getenv("DUNE_REPO_ROOT"))
	if root == "" {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			return repoPaths{}, fmt.Errorf("failed to locate repository root")
		}
		root = filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	}

	paths := repoPaths{
		Root:         root,
		Devcontainer: filepath.Join(root, "container", "devcontainer.json"),
		Manifest:     filepath.Join(root, "container", "gear", "manifest.tsv"),
	}
	if _, err := os.Stat(paths.Devcontainer); err != nil {
		return repoPaths{}, fmt.Errorf("expected devcontainer config at %s", paths.Devcontainer)
	}
	if _, err := os.Stat(paths.Manifest); err != nil {
		return repoPaths{}, fmt.Errorf("expected gear manifest at %s", paths.Manifest)
	}
	return paths, nil
}

func defaultWorkspaceInput(value string) string {
	if value != "" {
		return value
	}
	if caller := strings.TrimSpace(os.Getenv("DUNE_CALLER_PWD")); caller != "" {
		return caller
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}
