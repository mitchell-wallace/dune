package dune

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"claudebox/internal/dune/cli"
	"claudebox/internal/dune/pipelock"
	"claudebox/internal/dune/workspace"
	"claudebox/internal/version"
)

const (
	defaultProfile   = "default"
	composeShell     = "zsh"
	logTailLineCount = "60"
)

var (
	//go:embed compose.yaml.tmpl
	composeTemplateText string

	composeTemplate = template.Must(template.New("compose.yaml.tmpl").Parse(composeTemplateText))
	profileNameRE   = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

type Environment struct {
	CallerPWD string
}

type profileStore map[string]string

type project struct {
	WorkspaceRoot      string
	WorkspaceSlug      string
	Profile            string
	ComposeProject     string
	ComposeDir         string
	ComposePath        string
	PersistVolume      string
	BaseImage          string
	AgentImage         string
	UseBuild           bool
	PipelockImage      string
	PipelockConfigPath string
	TZ                 string
}

func Run(ctx context.Context, argv []string, env Environment, stdout, stderr io.Writer) error {
	opts, err := cli.Parse(argv)
	if err != nil {
		return err
	}

	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	workspaceInput := defaultWorkspaceInput(opts.WorkspaceInput, env.CallerPWD)
	ws, err := workspace.Resolve(workspaceInput)
	if err != nil {
		return err
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve config directory: %w", err)
	}
	dataHome, err := dataHomeDir()
	if err != nil {
		return err
	}

	storePath := filepath.Join(configDir, "dune", "profiles.json")
	store, err := loadProfileStore(storePath)
	if err != nil {
		return err
	}

	switch opts.Command {
	case cli.CommandVersion:
		_, err := fmt.Fprintf(stdout, "dune %s\n", version.String())
		return err
	case cli.CommandProfileSet:
		if err := validateProfileName(opts.SetProfileName); err != nil {
			return err
		}
		store[ws.Root] = opts.SetProfileName
		if err := saveProfileStore(storePath, store); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "%s -> %s\n", ws.Root, opts.SetProfileName)
		return nil
	case cli.CommandProfileList:
		return printProfileList(stdout, ws.Root, opts.Profile, opts.ProfileExplicit, store)
	}

	profile, err := resolveProfile(opts, ws.Root, store)
	if err != nil {
		return err
	}

	proj := project{
		WorkspaceRoot:      ws.Root,
		WorkspaceSlug:      ws.Slug,
		Profile:            profile,
		ComposeProject:     fmt.Sprintf("dune-%s-%s", ws.Slug, profile),
		ComposeDir:         filepath.Join(dataHome, "dune", "projects", ws.Slug),
		ComposePath:        filepath.Join(dataHome, "dune", "projects", ws.Slug, "compose.yaml"),
		PersistVolume:      "dune-persist-" + profile,
		BaseImage:          version.BaseImageRef(),
		AgentImage:         version.BaseImageRef(),
		UseBuild:           fileExists(filepath.Join(ws.Root, "Dockerfile.dune")),
		PipelockImage:      pipelock.ImageRef(),
		PipelockConfigPath: filepath.Join(configDir, "dune", "pipelock.yaml"),
		TZ:                 effectiveTimezone(),
	}
	if proj.UseBuild {
		proj.AgentImage = "dune-local-" + ws.Slug + ":latest"
	}

	switch opts.Command {
	case cli.CommandDown:
		if err := validateDockerPrerequisites(ctx); err != nil {
			return err
		}
		if err := ensureComposeFile(ctx, proj); err != nil {
			return err
		}
		return runStreaming(ctx, "", stdout, stderr, "docker", composeArgs(proj, "down")...)
	case cli.CommandLogs:
		if err := validateDockerPrerequisites(ctx); err != nil {
			return err
		}
		if err := ensureComposeFile(ctx, proj); err != nil {
			return err
		}
		args := append(composeArgs(proj, "logs", "-f"), opts.LogService)
		return runStreaming(ctx, "", stdout, stderr, "docker", compact(args)...)
	case cli.CommandRebuild:
		if err := validateDockerPrerequisites(ctx); err != nil {
			return err
		}
		if err := ensurePipelockConfig(ctx, proj.PipelockConfigPath); err != nil {
			return err
		}
		if err := ensureComposeFile(ctx, proj); err != nil {
			return err
		}
		if err := ensureVolume(ctx, proj.PersistVolume); err != nil {
			return err
		}
		if err := prepareAgentImage(ctx, proj, true, stdout, stderr); err != nil {
			return err
		}
		return runStreaming(ctx, "", stdout, stderr, "docker", composeArgs(proj, "up", "-d", "--force-recreate")...)
	case cli.CommandUp:
		if err := validateDockerPrerequisites(ctx); err != nil {
			return err
		}
		if err := ensurePipelockConfig(ctx, proj.PipelockConfigPath); err != nil {
			return err
		}
		if err := ensureComposeFile(ctx, proj); err != nil {
			return err
		}
		if err := ensureVolume(ctx, proj.PersistVolume); err != nil {
			return err
		}

		running, err := isAgentRunning(ctx, proj)
		if err != nil {
			return err
		}
		if !running {
			if err := prepareAgentImage(ctx, proj, false, stdout, stderr); err != nil {
				return err
			}
			if err := composeUp(ctx, proj, stderr); err != nil {
				return err
			}
		}
		return runStreaming(ctx, "", stdout, stderr, "docker", composeArgs(proj, "exec", "agent", composeShell)...)
	default:
		return fmt.Errorf("unsupported command %q", opts.Command)
	}
}

func resolveProfile(opts cli.Options, workspaceRoot string, store profileStore) (string, error) {
	if opts.ProfileExplicit {
		if err := validateProfileName(opts.Profile); err != nil {
			return "", err
		}
		return opts.Profile, nil
	}
	if stored := strings.TrimSpace(store[workspaceRoot]); stored != "" {
		return stored, nil
	}
	return defaultProfile, nil
}

func printProfileList(stdout io.Writer, workspaceRoot, explicit string, explicitSet bool, store profileStore) error {
	keys := make([]string, 0, len(store))
	for key := range store {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	effective := defaultProfile
	if explicitSet {
		effective = explicit
	} else if mapped := store[workspaceRoot]; mapped != "" {
		effective = mapped
	}

	if _, err := fmt.Fprintf(stdout, "Effective profile for %s: %s\n", workspaceRoot, effective); err != nil {
		return err
	}
	if len(keys) == 0 {
		_, err := fmt.Fprintln(stdout, "No stored profile mappings.")
		return err
	}

	for _, key := range keys {
		marker := " "
		if key == workspaceRoot {
			marker = "*"
		}
		if _, err := fmt.Fprintf(stdout, "%s %s -> %s\n", marker, key, store[key]); err != nil {
			return err
		}
	}
	return nil
}

func validateProfileName(name string) error {
	if !profileNameRE.MatchString(name) {
		return fmt.Errorf("invalid profile %q: use lowercase letters, numbers, and hyphens only", name)
	}
	return nil
}

func ensurePipelockConfig(ctx context.Context, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create pipelock config directory: %w", err)
	}

	var source []byte
	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		source = existing
	case os.IsNotExist(err):
		baseline, baselineErr := capture(ctx, "", "docker", pipelock.GenerateConfigCommand()[1:]...)
		if baselineErr != nil {
			return fmt.Errorf("generate pipelock baseline config: %w", baselineErr)
		}
		source = baseline
	default:
		return fmt.Errorf("read pipelock config: %w", err)
	}

	rendered, err := pipelock.ApplyCustomizations(source)
	if err != nil {
		return err
	}
	if bytes.Equal(existing, rendered) {
		return nil
	}
	if err := os.WriteFile(path, rendered, 0o644); err != nil {
		return fmt.Errorf("write pipelock config: %w", err)
	}
	return nil
}

func ensureComposeFile(ctx context.Context, proj project) error {
	if err := os.MkdirAll(proj.ComposeDir, 0o755); err != nil {
		return fmt.Errorf("create compose directory: %w", err)
	}
	rendered, err := renderComposeFile(proj)
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(proj.ComposeDir, "compose-*.yaml")
	if err != nil {
		return fmt.Errorf("create temporary compose file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(rendered); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temporary compose file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temporary compose file: %w", err)
	}

	if err := validateComposeFile(ctx, proj, tmpPath); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, proj.ComposePath); err != nil {
		return fmt.Errorf("write compose file: %w", err)
	}
	return nil
}

func renderComposeFile(proj project) ([]byte, error) {
	var rendered bytes.Buffer
	if err := composeTemplate.Execute(&rendered, proj); err != nil {
		return nil, fmt.Errorf("render compose template: %w", err)
	}
	return rendered.Bytes(), nil
}

func validateComposeFile(ctx context.Context, proj project, path string) error {
	args := []string{"compose", "-f", path, "-p", proj.ComposeProject, "config"}
	output, err := capture(ctx, "", "docker", args...)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return fmt.Errorf("validate compose file: %s", detail)
		}
		return fmt.Errorf("validate compose file: %w", err)
	}
	return nil
}

func ensureVolume(ctx context.Context, name string) error {
	if _, err := capture(ctx, "", "docker", "volume", "create", name); err != nil {
		return fmt.Errorf("create persist volume %q: %w", name, err)
	}
	return nil
}

func prepareAgentImage(ctx context.Context, proj project, noCache bool, stdout, stderr io.Writer) error {
	_, _ = fmt.Fprintf(stderr, "Pulling base image %s...\n", proj.BaseImage)
	if err := runStreaming(ctx, "", stdout, stderr, "docker", "pull", proj.BaseImage); err != nil {
		if !localImageExists(ctx, proj.BaseImage) {
			return fmt.Errorf("pull base image %q: %w", proj.BaseImage, err)
		}
		_, _ = fmt.Fprintf(stderr, "Base image pull failed, using existing local image %s.\n", proj.BaseImage)
	}
	if !proj.UseBuild {
		return nil
	}

	_, _ = fmt.Fprintln(stderr, "Building agent image from Dockerfile.dune...")
	args := composeArgs(proj, "build")
	if noCache {
		args = append(args, "--no-cache")
	}
	args = append(args, "agent")
	if err := runStreaming(ctx, "", stdout, stderr, "docker", args...); err != nil {
		return fmt.Errorf("build Dockerfile.dune image: %w", err)
	}
	return nil
}

func composeUp(ctx context.Context, proj project, stderr io.Writer) error {
	if _, err := capture(ctx, "", "docker", composeArgs(proj, "up", "-d")...); err != nil {
		tail, tailErr := capture(ctx, "", "docker", composeArgs(proj, "logs", "--tail", logTailLineCount)...)
		if tailErr == nil && len(bytes.TrimSpace(tail)) > 0 {
			_, _ = fmt.Fprintf(stderr, "Recent compose logs:\n%s\n", tail)
		}
		return fmt.Errorf("docker compose up failed: %w", err)
	}
	return nil
}

func isAgentRunning(ctx context.Context, proj project) (bool, error) {
	output, err := capture(ctx, "", "docker", composeArgs(proj, "ps", "--status", "running", "--services", "agent")...)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) == 0 {
			return false, nil
		}
		return false, fmt.Errorf("inspect compose service state: %w", err)
	}
	return strings.TrimSpace(string(output)) == "agent", nil
}

func validateDockerPrerequisites(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("docker is not installed or not on PATH")
	}
	if _, err := capture(ctx, "", "docker", "compose", "version"); err != nil {
		return fmt.Errorf("docker compose is unavailable: %w", err)
	}
	if _, err := capture(ctx, "", "docker", "info"); err != nil {
		return fmt.Errorf("docker daemon is not reachable; start Docker and try again: %w", err)
	}
	return nil
}

func composeArgs(proj project, args ...string) []string {
	base := []string{"compose", "-f", proj.ComposePath, "-p", proj.ComposeProject}
	return append(base, args...)
}

func localImageExists(ctx context.Context, image string) bool {
	output, err := capture(ctx, "", "docker", "image", "inspect", image)
	return err == nil && len(output) > 0
}

func capture(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitErr.Stderr = output
		}
		return output, err
	}
	return output, nil
}

func runStreaming(ctx context.Context, dir string, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func loadProfileStore(path string) (profileStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return profileStore{}, nil
		}
		return nil, fmt.Errorf("read profile mappings: %w", err)
	}

	var store profileStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parse profile mappings: %w", err)
	}
	if store == nil {
		store = profileStore{}
	}
	return store, nil
}

func saveProfileStore(path string, store profileStore) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create profile mapping directory: %w", err)
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile mappings: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write profile mappings: %w", err)
	}
	return nil
}

func defaultWorkspaceInput(explicit, callerPWD string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if strings.TrimSpace(callerPWD) != "" {
		return callerPWD
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func effectiveTimezone() string {
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" {
		return tz
	}
	return "UTC"
}

func dataHomeDir() (string, error) {
	if home := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share"), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func compact(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			result = append(result, item)
		}
	}
	return result
}
