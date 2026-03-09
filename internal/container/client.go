package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"claudebox/internal/config"
	"claudebox/internal/domain"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
	Output(ctx context.Context, name string, args ...string) (string, error)
	CombinedOutput(ctx context.Context, name string, args ...string) (string, error)
	Interactive(ctx context.Context, name string, args ...string) error
}

type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func (OSRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.Output()
	return strings.TrimSpace(string(output)), err
}

func (OSRunner) CombinedOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func (OSRunner) Interactive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type Client struct {
	Runner CommandRunner
}

func NewClient(runner CommandRunner) *Client {
	return &Client{Runner: runner}
}

func (c *Client) ContainerExists(ctx context.Context, name string) bool {
	return c.Runner.Run(ctx, "docker", "container", "inspect", name) == nil
}

func (c *Client) ContainerRunning(ctx context.Context, name string) bool {
	output, err := c.Runner.Output(ctx, "docker", "inspect", "-f", "{{.State.Running}}", name)
	return err == nil && output == "true"
}

func (c *Client) ContainerEnvValue(ctx context.Context, name, key string) (string, error) {
	return c.Runner.Output(ctx, "docker", "inspect", "-f", fmt.Sprintf("{{range .Config.Env}}{{println .}}{{end}}"), name)
}

func (c *Client) ResolveContainerMode(ctx context.Context, name string) (domain.Mode, error) {
	if c.ContainerRunning(ctx, name) {
		output, err := c.Runner.Output(ctx, "docker", "exec", name, "sh", "-lc", "cat /etc/sand/security-mode 2>/dev/null || true")
		if err == nil {
			if mode, ok := config.CanonicalizeMode(strings.TrimSpace(output)); ok {
				return mode, nil
			}
		}
	}

	envValue, err := c.Runner.Output(ctx, "docker", "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", name)
	if err != nil {
		return domain.ModeStd, nil
	}
	for _, line := range strings.Split(envValue, "\n") {
		if strings.HasPrefix(line, "SAND_SECURITY_MODE=") {
			if mode, ok := config.CanonicalizeMode(strings.TrimPrefix(line, "SAND_SECURITY_MODE=")); ok {
				return mode, nil
			}
		}
	}

	return domain.ModeStd, nil
}

func (c *Client) ResolveWorkspaceMode(ctx context.Context, name string) (domain.WorkspaceMode, error) {
	envValue, err := c.Runner.Output(ctx, "docker", "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", name)
	if err != nil {
		return domain.WorkspaceModeMount, nil
	}
	for _, line := range strings.Split(envValue, "\n") {
		if strings.HasPrefix(line, "SAND_WORKSPACE_MODE=") {
			if mode, ok := config.NormalizeWorkspaceMode(strings.TrimPrefix(line, "SAND_WORKSPACE_MODE=")); ok {
				return mode, nil
			}
		}
	}
	return domain.WorkspaceModeMount, nil
}

func (c *Client) RemoveContainer(ctx context.Context, name string) error {
	return c.Runner.Run(ctx, "docker", "rm", "-f", name)
}

func (c *Client) RenameContainer(ctx context.Context, from, to string) error {
	return c.Runner.Run(ctx, "docker", "rename", from, to)
}

func (c *Client) StartContainer(ctx context.Context, name string) error {
	return c.Runner.Run(ctx, "docker", "start", name)
}

func (c *Client) FindCreatedContainerID(ctx context.Context, workspaceDir, configPath, profile string) (string, error) {
	return c.Runner.Output(ctx, "docker", "ps", "-aq",
		"--filter", "label=devcontainer.local_folder="+workspaceDir,
		"--filter", "label=devcontainer.config_file="+configPath,
		"--filter", "label=sand.profile="+profile,
	)
}

func (c *Client) ContainerName(ctx context.Context, id string) (string, error) {
	name, err := c.Runner.Output(ctx, "docker", "inspect", "-f", "{{.Name}}", id)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(name, "/"), nil
}

func (c *Client) CopyWorkspace(ctx context.Context, workspaceDir, containerName string) error {
	if !c.ContainerRunning(ctx, containerName) {
		if err := c.StartContainer(ctx, containerName); err != nil {
			return err
		}
	}
	if err := c.Runner.Run(ctx, "docker", "cp", workspaceDir+"/.", containerName+":/workspace/"); err != nil {
		return err
	}
	return c.Runner.Run(ctx, "docker", "exec", "--user", "root", containerName, "chown", "-R", "node:node", "/workspace")
}

func (c *Client) ContainerFileExists(ctx context.Context, name, path string) bool {
	return c.Runner.Run(ctx, "docker", "exec", name, "sh", "-lc", "[ -f "+shellQuote(path)+" ]") == nil
}

func (c *Client) ExecInContainer(ctx context.Context, name string, env map[string]string, args ...string) error {
	command := []string{"exec"}
	for key, value := range env {
		command = append(command, "-e", key+"="+value)
	}
	command = append(command, name)
	command = append(command, args...)
	return c.Runner.Run(ctx, "docker", command...)
}

func (c *Client) AttachShell(ctx context.Context, name string) error {
	return c.Runner.Interactive(ctx, "docker", "exec", "-it", name, "zsh")
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}
