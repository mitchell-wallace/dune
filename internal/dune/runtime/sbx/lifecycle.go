package sbx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultShell = "zsh"

func (b *backend) Ensure(ctx context.Context, spec Spec) error {
	if err := validateEnsureSpec(spec); err != nil {
		return err
	}

	state, err := b.Status(ctx, spec)
	if err != nil {
		return err
	}
	if state.Exists {
		return nil
	}

	persistHostPath, err := ensureProfilePersistDir(spec.Profile)
	if err != nil {
		return err
	}

	if _, err := b.runner.Capture(ctx, "", "sbx", "create",
		"--name", spec.InstanceName,
		"--template", spec.TemplateRef,
		"shell",
		spec.WorkspaceHostPath,
		persistHostPath,
	); err != nil {
		return fmt.Errorf("create sbx sandbox %q: %w", spec.InstanceName, err)
	}

	if _, err := b.runner.Capture(ctx, "", "sbx", "exec",
		"-e", "DUNE_WORKSPACE="+spec.WorkspaceHostPath,
		"-e", "PERSIST_DIR="+persistHostPath,
		spec.InstanceName,
		"bash", "-lc", "true",
	); err != nil {
		return fmt.Errorf("initialise sbx sandbox %q: %w", spec.InstanceName, err)
	}

	return nil
}

func (b *backend) Start(ctx context.Context, spec Spec) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}
	if _, err := b.runner.Capture(ctx, "", "sbx", "run", spec.InstanceName); err != nil {
		return fmt.Errorf("start sbx sandbox %q: %w", spec.InstanceName, err)
	}
	return nil
}

func (b *backend) Shell(ctx context.Context, spec Spec, streams StdIO) error {
	if err := validateShellSpec(spec); err != nil {
		return err
	}

	args := shellExecArgs(spec, true)
	err := b.runner.Stream(ctx, "", streams, "sbx", args...)
	if !isUnsupportedWorkingDirFlag(err) {
		return err
	}

	return b.runner.Stream(ctx, "", streams, "sbx", shellExecArgs(spec, false)...)
}

func (b *backend) Stop(ctx context.Context, spec Spec) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}
	if _, err := b.runner.Capture(ctx, "", "sbx", "stop", spec.InstanceName); err != nil {
		return fmt.Errorf("stop sbx sandbox %q: %w", spec.InstanceName, err)
	}
	return nil
}

func (b *backend) Destroy(ctx context.Context, spec Spec) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}
	if _, err := b.runner.Capture(ctx, "", "sbx", "rm", "--force", spec.InstanceName); err != nil {
		return fmt.Errorf("remove sbx sandbox %q: %w", spec.InstanceName, err)
	}
	return nil
}

func (b *backend) Logs(ctx context.Context, spec Spec, service string, streams StdIO) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}

	script, err := logsScript(service)
	if err != nil {
		return err
	}

	if err := b.runner.Stream(ctx, "", streams, "sbx", "exec", spec.InstanceName, "bash", "-lc", script); err != nil {
		return fmt.Errorf("stream sbx logs for sandbox %q: %w", spec.InstanceName, err)
	}
	return nil
}

func (b *backend) Rebuild(ctx context.Context, spec Spec, streams StdIO) error {
	if err := validateEnsureSpec(spec); err != nil {
		return err
	}
	state, err := b.Status(ctx, spec)
	if err != nil {
		return err
	}
	if state.Exists {
		if err := b.Destroy(ctx, spec); err != nil {
			return err
		}
	}
	if err := b.Ensure(ctx, spec); err != nil {
		return err
	}
	if err := b.VerifyEgressPosture(ctx, spec, streams); err != nil {
		return err
	}
	if err := b.Start(ctx, spec); err != nil {
		return err
	}
	return nil
}

func (b *backend) Status(ctx context.Context, spec Spec) (State, error) {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return State{}, err
	}

	output, err := b.runner.Capture(ctx, "", "sbx", "ls", "--json")
	if err != nil {
		return State{}, fmt.Errorf("list sbx sandboxes: %w", err)
	}

	sandboxes, err := parseSandboxList(output)
	if err != nil {
		return State{}, fmt.Errorf("parse sbx ls output: %w", err)
	}

	for _, sandbox := range sandboxes {
		if sandbox.Name == spec.InstanceName {
			return State{
				Exists:  true,
				Running: isRunningStatus(sandbox.Status),
			}, nil
		}
	}
	return State{}, nil
}

type sandboxListEntry struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func parseSandboxList(output []byte) ([]sandboxListEntry, error) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return nil, errors.New("empty sbx ls JSON")
	}

	switch trimmed[0] {
	case '[':
		var entries []sandboxListEntry
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return nil, err
		}
		return entries, nil
	case '{':
		var raw struct {
			Sandboxes json.RawMessage `json:"sandboxes"`
		}
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			return nil, err
		}
		if len(raw.Sandboxes) == 0 {
			return nil, errors.New(`missing "sandboxes" field`)
		}
		var entries []sandboxListEntry
		if err := json.Unmarshal(raw.Sandboxes, &entries); err != nil {
			return nil, err
		}
		return entries, nil
	default:
		return nil, fmt.Errorf("unexpected JSON token %q", trimmed[0])
	}
}

func isRunningStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "running")
}

func shellExecArgs(spec Spec, withWorkingDirFlag bool) []string {
	shell := shellName(spec)
	args := []string{"exec", "-it"}
	args = append(args, hostTerminalEnvArgs()...)
	if withWorkingDirFlag {
		args = append(args, "-w", workingDir(spec), spec.InstanceName, shell)
		return args
	}
	args = append(args, spec.InstanceName, shell, "-lc", "cd "+shellQuote(workingDir(spec))+" && exec "+shellQuote(shell)+" -l")
	return args
}

func hostTerminalEnvArgs() []string {
	var args []string
	for _, key := range []string{"TERM", "COLORTERM"} {
		if value := os.Getenv(key); value != "" {
			args = append(args, "-e", key+"="+value)
		}
	}
	return args
}

func isUnsupportedWorkingDirFlag(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown shorthand flag") && strings.Contains(msg, "w") ||
		strings.Contains(msg, "unknown flag: -w") ||
		strings.Contains(msg, "flag provided but not defined: -w")
}

func validateEnsureSpec(spec Spec) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}
	if strings.TrimSpace(spec.TemplateRef) == "" {
		return errors.New("sbx template ref is required")
	}
	if err := validateAbsolutePath("workspace host path", spec.WorkspaceHostPath); err != nil {
		return err
	}
	if strings.TrimSpace(spec.Profile) == "" {
		return errors.New("profile is required")
	}
	return nil
}

func validateShellSpec(spec Spec) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}
	return validateAbsolutePath("working directory", workingDir(spec))
}

func validateInstanceName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("instance name is required")
	}
	return nil
}

func validateAbsolutePath(label, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s is required", label)
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s must be absolute: %s", label, path)
	}
	return nil
}

func ensureProfilePersistDir(profile string) (string, error) {
	path, err := profilePersistHostPath(profile)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", fmt.Errorf("create profile persist directory %q: %w", path, err)
	}
	return path, nil
}

func profilePersistHostPath(profile string) (string, error) {
	if strings.TrimSpace(profile) == "" {
		return "", errors.New("profile is required")
	}

	dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}

	path := filepath.Join(dataHome, "dune", "persist", profile)
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve profile persist directory: %w", err)
		}
		path = abs
	}
	return path, nil
}

func workingDir(spec Spec) string {
	if strings.TrimSpace(spec.WorkingDir) != "" {
		return spec.WorkingDir
	}
	return spec.WorkspaceHostPath
}

func shellName(spec Spec) string {
	if strings.TrimSpace(spec.Shell) != "" {
		return spec.Shell
	}
	return defaultShell
}

func logsScript(service string) (string, error) {
	service = strings.TrimSpace(service)
	if service == "" || service == "all" {
		return "if compgen -G '/var/log/dune/*.log' >/dev/null; then tail -n +1 -f /var/log/dune/*.log; else echo 'No Dune logs found under /var/log/dune'; fi", nil
	}
	if !validLogServiceName(service) {
		return "", fmt.Errorf("invalid log service %q: use letters, numbers, dots, underscores, or hyphens", service)
	}

	path := "/var/log/dune/" + service + ".log"
	return "if [ -f " + shellQuote(path) + " ]; then tail -n +1 -f " + shellQuote(path) + "; else echo " + shellQuote("No Dune log found for "+service+" at "+path) + "; fi", nil
}

func validLogServiceName(service string) bool {
	if service == "" {
		return false
	}
	for _, r := range service {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
