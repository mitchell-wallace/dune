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

	"claudebox/internal/dune/cli"
	"claudebox/internal/dune/config"
	"claudebox/internal/dune/container"
	"claudebox/internal/dune/devcontainer"
	"claudebox/internal/dune/domain"
	"claudebox/internal/dune/gear"
	"claudebox/internal/dune/rallysync"
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

type rallyContainer interface {
	ContainerExists(ctx context.Context, name string) bool
	ContainerRunning(ctx context.Context, name string) bool
	StartContainer(ctx context.Context, name string) error
	ContainerMounts(ctx context.Context, name string) ([]container.Mount, error)
	ExecInContainer(ctx context.Context, name string, env map[string]string, args ...string) error
	ExecInContainerAsUser(ctx context.Context, name, user string, env map[string]string, args ...string) error
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
	case cli.CommandRallyUpdate:
		return runRallyUpdate(context.Background(), opts, paths)
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
	fmt.Println("Building rally binary...")
	systemBinary, err := rallysync.HostSystemBinaryPath()
	if err != nil {
		return err
	}
	if err := buildRallyBinary(ctx, paths.Root, systemBinary); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}
	ver, commit := standaloneRallyBuildInfo(ctx, paths.Root)
	fmt.Printf("Built rally %s (%s) at %s\n", ver, commit, systemBinary)

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
		fmt.Printf("Updated system rally binary; container %s does not exist yet\n", identity.Name)
		return nil
	}

	if err := syncRallyBinaryToContainer(ctx, docker, container.OSRunner{}, identity.Name, systemBinary); err != nil {
		return err
	}
	fmt.Printf("Pushed rally %s (%s) to %s\n", ver, commit, identity.Name)
	return nil
}

func runRallyUpdate(ctx context.Context, opts cli.Options, paths repoPaths) error {
	systemBinary, err := rallysync.HostSystemBinaryPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(systemBinary); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("system rally binary not found at %s; run `dune rally build` first", systemBinary)
		}
		return err
	}

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

	if err := syncRallyBinaryToContainer(ctx, docker, container.OSRunner{}, identity.Name, systemBinary); err != nil {
		return err
	}

	ver, commit := standaloneRallyBuildInfo(ctx, paths.Root)
	fmt.Printf("Updated %s with system rally %s (%s)\n", identity.Name, ver, commit)
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
	systemBinary, err := ensureRallySystemBinary(ctx, paths.Root)
	if err != nil {
		return err
	}

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

		if err := injectRallyRuntimeConfig(effectiveConfig, identity.Name, cfg.Beads); err != nil {
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

	if err := syncRallyBinaryToContainer(ctx, docker, container.OSRunner{}, identity.Name, systemBinary); err != nil {
		return err
	}

	if err := applyConfiguredGear(ctx, docker, cfg, identity.Name, paths.Manifest); err != nil {
		return err
	}

	return docker.AttachShell(ctx, identity.Name)
}

func buildRallyBinary(ctx context.Context, repoRoot, binPath string) error {
	rallyRepoRoot, err := locateStandaloneRallyRepo(repoRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		return err
	}

	ver := readVersion(rallyRepoRoot)
	commit := gitCommitShort(ctx, rallyRepoRoot)
	if ver == "" {
		ver = "dev"
	}

	cmdArgs := rallysync.HostBinaryBuildCommandForPath(rallyRepoRoot, binPath, ver, commit)
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = rallyRepoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	return cmd.Run()
}

func ensureRallySystemBinary(ctx context.Context, repoRoot string) (string, error) {
	binPath, err := rallysync.HostSystemBinaryPath()
	if err != nil {
		return "", err
	}
	rallyRepoRoot, err := locateStandaloneRallyRepo(repoRoot)
	if err != nil {
		return "", err
	}
	if !rallyBinaryNeedsBuild(rallyRepoRoot, binPath) {
		return binPath, nil
	}
	fmt.Println("Building rally system binary...")
	if err := buildRallyBinary(ctx, repoRoot, binPath); err != nil {
		return "", fmt.Errorf("build failed: %w", err)
	}
	return binPath, nil
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

func injectRallyRuntimeConfig(configPath, containerName, beadsMode string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	envMap := asMapAny(cfg["containerEnv"])
	for key, value := range rallysync.ContainerEnv(containerName) {
		envMap[key] = value
	}
	if beadsMode != "" {
		envMap[rallysync.EnvBeads] = beadsMode
	}
	cfg["containerEnv"] = envMap

	rendered, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, append(rendered, '\n'), 0o644)
}

func syncRallyBinaryToContainer(ctx context.Context, docker rallyContainer, runner container.CommandRunner, containerName, hostBinary string) error {
	if !docker.ContainerExists(ctx, containerName) {
		return fmt.Errorf("container %s does not exist", containerName)
	}
	if !docker.ContainerRunning(ctx, containerName) {
		fmt.Printf("Starting container %s...\n", containerName)
		if err := docker.StartContainer(ctx, containerName); err != nil {
			return err
		}
	}

	mounts, err := docker.ContainerMounts(ctx, containerName)
	if err != nil {
		return err
	}
	var legacyBinarySource string
	for _, mount := range mounts {
		if mount.Destination == rallysync.ContainerBinaryPath {
			legacyBinarySource = mount.Source
			break
		}
	}

	if err := docker.ExecInContainer(ctx, containerName, nil, "mkdir", "-p", rallysync.PersistentBinaryDir); err != nil {
		return fmt.Errorf("failed to create %s in container: %w", rallysync.PersistentBinaryDir, err)
	}

	tmpPath := rallysync.PersistentBinaryPath + ".tmp"
	if err := runner.Run(ctx, "docker", "cp", hostBinary, containerName+":"+tmpPath); err != nil {
		return fmt.Errorf("docker cp failed: %w", err)
	}
	if err := docker.ExecInContainerAsUser(ctx, containerName, "root", nil, "sh", "-lc",
		fmt.Sprintf("install -m 0755 %s %s && rm -f %s && /usr/local/bin/setup-agent-persist.sh",
			shellQuote(tmpPath), shellQuote(rallysync.PersistentBinaryPath), shellQuote(tmpPath))); err != nil {
		return fmt.Errorf("failed to activate rally in container: %w", err)
	}
	if legacyBinarySource != "" {
		legacyBinarySourceHostPath := normalizeDockerHostPath(legacyBinarySource)
		if err := os.MkdirAll(filepath.Dir(legacyBinarySourceHostPath), 0o755); err != nil {
			return fmt.Errorf("failed to prepare legacy rally bind mount source %s: %w", legacyBinarySourceHostPath, err)
		}
		if err := runner.Run(ctx, "install", "-m", "0755", hostBinary, legacyBinarySourceHostPath); err != nil {
			return fmt.Errorf("failed to update legacy rally bind mount source %s: %w", legacyBinarySourceHostPath, err)
		}
		fmt.Fprintf(os.Stderr, "WARNING: container %s still bind-mounts %s; updated legacy source %s for compatibility, but run `dune rebuild` to migrate to persisted rally updates\n", containerName, rallysync.ContainerBinaryPath, legacyBinarySourceHostPath)
	}
	return nil
}

func normalizeDockerHostPath(path string) string {
	if strings.HasPrefix(path, "/host_mnt/") {
		return strings.TrimPrefix(path, "/host_mnt")
	}
	return path
}

func rallyBinaryNeedsBuild(repoRoot, binPath string) bool {
	info, err := os.Stat(binPath)
	if err != nil {
		return true
	}
	binModTime := info.ModTime()
	for _, input := range rallyBuildInputs(repoRoot) {
		inputInfo, err := os.Stat(input)
		if err == nil && inputInfo.ModTime().After(binModTime) {
			return true
		}
	}
	return false
}

func rallyBuildInputs(repoRoot string) []string {
	paths := []string{
		filepath.Join(repoRoot, "go.mod"),
		filepath.Join(repoRoot, "go.sum"),
		filepath.Join(repoRoot, "VERSION"),
	}
	for _, dirName := range []string{"cmd", "internal"} {
		root := filepath.Join(repoRoot, dirName)
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, ".go") {
				paths = append(paths, path)
			}
			return nil
		})
	}
	return paths
}

func locateStandaloneRallyRepo(repoRoot string) (string, error) {
	path := filepath.Clean(filepath.Join(repoRoot, "..", "rally"))
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("standalone rally repo not found at %s", path)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("standalone rally repo path is not a directory: %s", path)
	}
	return path, nil
}

func standaloneRallyBuildInfo(ctx context.Context, repoRoot string) (string, string) {
	rallyRepoRoot, err := locateStandaloneRallyRepo(repoRoot)
	if err != nil {
		return "dev", ""
	}
	ver := readVersion(rallyRepoRoot)
	if ver == "" {
		ver = "dev"
	}
	return ver, gitCommitShort(ctx, rallyRepoRoot)
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

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
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
