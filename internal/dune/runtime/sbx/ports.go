package sbx

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ListPorts lists the published ports for the instance's sandbox, constructing
// the verified spike-4 shape `sbx ports <sandbox>` through the sbx-3 runner
// seam. `sbx ports <sandbox>` prints a human-readable list of published ports;
// the raw output is returned for the caller to surface verbatim (mirrors
// ListServiceSecrets). The per-command `sbx ports <sandbox> --json` form exists
// (sbx-3 D6: JSON flags are not uniform across sbx verbs), but Dune surfaces
// the human-readable list by default and does not assume a uniform --json.
func (b *backend) ListPorts(ctx context.Context, spec Spec) ([]byte, error) {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return nil, err
	}
	output, err := b.runner.Capture(ctx, "", "sbx", "ports", spec.InstanceName)
	if err != nil {
		return output, WrapCommandError(CodeSbxDiagnoseFailed, "list sbx ports for sandbox "+spec.InstanceName+" failed", commandResult("sbx", []string{"ports", spec.InstanceName}, output, err), err)
	}
	return output, nil
}

// PublishPorts publishes host->sandbox port mappings, constructing the verified
// spike-4 shape `sbx ports <sandbox> --publish <spec>` (repeatable: one
// --publish flag per spec) through the sbx-3 runner seam. Each spec follows
// [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]; an omitted HOST_PORT yields an
// ephemeral host port and an omitted HOST_IP binds to loopback.
func (b *backend) PublishPorts(ctx context.Context, spec Spec, specs []string) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}
	args, err := portsFlagArgs(spec.InstanceName, "--publish", specs)
	if err != nil {
		return err
	}
	output, err := b.runner.Capture(ctx, "", "sbx", args...)
	if err != nil {
		return WrapCommandError(CodeSbxExecFailed, "publish sbx ports for sandbox "+spec.InstanceName+" failed", commandResult("sbx", args, output, err), err)
	}
	return nil
}

// UnpublishPorts removes host->sandbox port mappings, constructing the verified
// spike-4 shape `sbx ports <sandbox> --unpublish <spec>` (repeatable: one
// --unpublish flag per spec) through the sbx-3 runner seam. Each spec follows
// [HOST_IP:]HOST_PORT:SANDBOX_PORT[/PROTOCOL].
func (b *backend) UnpublishPorts(ctx context.Context, spec Spec, specs []string) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}
	args, err := portsFlagArgs(spec.InstanceName, "--unpublish", specs)
	if err != nil {
		return err
	}
	output, err := b.runner.Capture(ctx, "", "sbx", args...)
	if err != nil {
		return WrapCommandError(CodeSbxExecFailed, "unpublish sbx ports for sandbox "+spec.InstanceName+" failed", commandResult("sbx", args, output, err), err)
	}
	return nil
}

// portsFlagArgs builds the `sbx ports <instance> <flag> <spec>...` argument
// vector, validating every spec first so a malformed spec cannot reshape the
// constructed invocation (mirrors validateSecretService in secrets.go).
func portsFlagArgs(instanceName, flag string, specs []string) ([]string, error) {
	if len(specs) == 0 {
		return nil, errors.New("at least one port spec is required")
	}
	for _, spec := range specs {
		if err := validatePortSpec(spec); err != nil {
			return nil, err
		}
	}
	args := []string{"ports", instanceName}
	for _, spec := range specs {
		args = append(args, flag, spec)
	}
	return args, nil
}

// validatePortSpec guards a --publish/--unpublish port spec. The verified
// spike-4 shape passes a single port-spec token (e.g. "8080", "3000:8080",
// "127.0.0.1:3000:8080/tcp"); it must not be empty and must not carry shell/
// flag metacharacters that could reshape the constructed sbx invocation. A
// conservative character set (digits, letters, '.', ':', '/') keeps the spec to
// plausible HOST_IP:HOST_PORT:SANDBOX_PORT[/PROTOCOL] forms.
func validatePortSpec(spec string) error {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return errors.New("port spec is required")
	}
	if strings.ContainsAny(spec, " \t\r\n") {
		return fmt.Errorf("port spec must be a single token: %q", spec)
	}
	if strings.HasPrefix(spec, "-") {
		return fmt.Errorf("port spec must not look like a flag: %q", spec)
	}
	for _, r := range spec {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '.' || r == ':' || r == '/':
		default:
			return fmt.Errorf("port spec %q contains invalid character %q", spec, r)
		}
	}
	return nil
}
