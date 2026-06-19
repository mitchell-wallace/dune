package sbx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
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

	createArgs := []string{
		"create",
		"--name", spec.InstanceName,
		"--template", spec.TemplateRef,
		"shell",
		spec.WorkspaceHostPath,
		persistHostPath,
	}
	createOutput, err := b.runner.Capture(ctx, "", "sbx", createArgs...)
	if err != nil {
		b.writeLifecycleLog(spec, "create failed: "+err.Error())
		return WrapCommandError(createFailureCode(createOutput), "create sbx sandbox "+spec.InstanceName+" failed", commandResult("sbx", createArgs, createOutput, err), err)
	}
	b.writeLifecycleLog(spec, "created sandbox from template "+spec.TemplateRef)

	setupArgs := []string{
		"exec",
		"-e", "DUNE_WORKSPACE=" + spec.WorkspaceHostPath,
		"-e", "PERSIST_DIR=" + persistHostPath,
		spec.InstanceName,
		"bash", "-lc", "true",
	}
	setupOutput, err := b.runner.Capture(ctx, "", "sbx", setupArgs...)
	if err != nil {
		b.writeLifecycleLog(spec, "setup hook failed: "+err.Error())
		return WrapCommandError(CodeSbxExecFailed, "initialise sbx sandbox "+spec.InstanceName+" failed", commandResult("sbx", setupArgs, setupOutput, err), err)
	}
	b.writeLifecycleLog(spec, "ran setup hook for workspace "+spec.WorkspaceHostPath)

	return nil
}

func (b *backend) Start(ctx context.Context, spec Spec) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}
	args := []string{"run", spec.InstanceName}
	output, err := b.runner.Capture(ctx, "", "sbx", args...)
	if err != nil {
		b.writeLifecycleLog(spec, "start failed: "+err.Error())
		return WrapCommandError(CodeSbxStartFailed, "start sbx sandbox "+spec.InstanceName+" failed", commandResult("sbx", args, output, err), err)
	}
	b.writeLifecycleLog(spec, "started sandbox")
	return nil
}

func (b *backend) Shell(ctx context.Context, spec Spec, streams StdIO) error {
	if err := validateShellSpec(spec); err != nil {
		return err
	}

	args := shellExecArgs(spec, true)
	b.writeLifecycleLog(spec, "attaching shell at "+workingDir(spec))
	err := b.runner.Stream(ctx, "", streams, "sbx", args...)
	if !isUnsupportedWorkingDirFlag(err) {
		if err != nil {
			b.writeLifecycleLog(spec, "attach failed: "+err.Error())
			return WrapCommandError(CodeSbxExecFailed, "attach to sbx sandbox "+spec.InstanceName+" failed", commandResult("sbx", args, nil, err), err)
		}
		return nil
	}

	b.writeLifecycleLog(spec, "retrying shell attach without sbx -w support")
	args = shellExecArgs(spec, false)
	if err := b.runner.Stream(ctx, "", streams, "sbx", args...); err != nil {
		b.writeLifecycleLog(spec, "attach retry failed: "+err.Error())
		return WrapCommandError(CodeSbxExecFailed, "attach to sbx sandbox "+spec.InstanceName+" failed", commandResult("sbx", args, nil, err), err)
	}
	return nil
}

func (b *backend) Stop(ctx context.Context, spec Spec) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}
	args := []string{"stop", spec.InstanceName}
	output, err := b.runner.Capture(ctx, "", "sbx", args...)
	if err != nil {
		return WrapCommandError(CodeSbxStopFailed, "stop sbx sandbox "+spec.InstanceName+" failed", commandResult("sbx", args, output, err), err)
	}
	return nil
}

func (b *backend) Destroy(ctx context.Context, spec Spec) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}
	args := []string{"rm", "--force", spec.InstanceName}
	output, err := b.runner.Capture(ctx, "", "sbx", args...)
	if err != nil {
		return WrapCommandError(CodeSbxRmFailed, "remove sbx sandbox "+spec.InstanceName+" failed", commandResult("sbx", args, output, err), err)
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

	stdout := streams.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	if err := writeHostLifecycleLog(stdout, spec); err != nil {
		return err
	}

	args := []string{"exec", spec.InstanceName, "bash", "-lc", script}
	output, err := b.runner.Capture(ctx, "", "sbx", args...)
	if err != nil {
		return WrapCommandError(CodeSbxExecFailed, "read in-sandbox Dune logs for sandbox "+spec.InstanceName+" failed", commandResult("sbx", args, output, err), err)
	}
	if _, err := fmt.Fprint(stdout, "\n== dune sandbox logs (/var/log/dune) ==\n"); err != nil {
		return fmt.Errorf("write sandbox log heading: %w", err)
	}
	if _, err := stdout.Write(output); err != nil {
		return fmt.Errorf("write sandbox logs: %w", err)
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

	args := []string{"ls", "--json"}
	output, err := b.runner.Capture(ctx, "", "sbx", args...)
	if err != nil {
		return State{}, WrapCommandError(CodeSbxDiagnoseFailed, "list sbx sandboxes failed", commandResult("sbx", args, output, err), err)
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
		return NewDiagnosticError(CodeTemplateUnavailable, "sbx template ref is required", "", errors.New("sbx template ref is required"))
	}
	if err := validateAbsolutePath("workspace host path", spec.WorkspaceHostPath); err != nil {
		return NewDiagnosticError(CodeWorkspaceInvalid, "workspace host path is invalid", err.Error(), err)
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
	if err := validateAbsolutePath("working directory", workingDir(spec)); err != nil {
		return NewDiagnosticError(CodeWorkspaceInvalid, "working directory is invalid", err.Error(), err)
	}
	return nil
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

func ProfilePersistHostPath(profile string) (string, error) {
	return profilePersistHostPath(profile)
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
		return "if compgen -G '/var/log/dune/*.log' >/dev/null; then for f in /var/log/dune/*.log; do echo '---' \"$f\"; cat \"$f\"; done; else echo 'No Dune logs found under /var/log/dune'; fi", nil
	}
	if !validLogServiceName(service) {
		return "", fmt.Errorf("invalid log service %q: use letters, numbers, dots, underscores, or hyphens", service)
	}

	path := "/var/log/dune/" + service + ".log"
	return "if [ -f " + shellQuote(path) + " ]; then cat " + shellQuote(path) + "; else echo " + shellQuote("No Dune log found for "+service+" at "+path) + "; fi", nil
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

func createFailureCode(output []byte) string {
	text := strings.ToLower(string(output))
	if strings.Contains(text, "template") || strings.Contains(text, "pull") || strings.Contains(text, "image") {
		return CodeTemplateUnavailable
	}
	return CodeSbxCreateFailed
}

func (b *backend) writeLifecycleLog(spec Spec, message string) {
	path, err := lifecycleLogPath(spec)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	line := fmt.Sprintf("%s %s\n", time.Now().UTC().Format(time.RFC3339), message)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = file.WriteString(line)
}

func writeHostLifecycleLog(w io.Writer, spec Spec) error {
	if _, err := fmt.Fprint(w, "== dune host lifecycle log ==\n"); err != nil {
		return fmt.Errorf("write host lifecycle log heading: %w", err)
	}
	path, err := lifecycleLogPath(spec)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			_, err = fmt.Fprintf(w, "No Dune host lifecycle log found at %s\n", path)
			return err
		}
		return fmt.Errorf("read host lifecycle log %q: %w", path, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write host lifecycle log: %w", err)
	}
	return nil
}

func lifecycleLogPath(spec Spec) (string, error) {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return "", err
	}
	stateHome := strings.TrimSpace(os.Getenv("XDG_STATE_HOME"))
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	if !filepath.IsAbs(stateHome) {
		abs, err := filepath.Abs(stateHome)
		if err != nil {
			return "", fmt.Errorf("resolve state directory: %w", err)
		}
		stateHome = abs
	}
	return filepath.Join(stateHome, "dune", "logs", spec.InstanceName+".log"), nil
}
