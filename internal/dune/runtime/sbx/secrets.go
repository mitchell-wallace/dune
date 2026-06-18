package sbx

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// SetServiceSecret stores a service-identifier secret scoped to the instance's
// sandbox, constructing the verified spike-3 shape `sbx secret set <instance>
// <service> -t <token>` through the sbx-3 runner seam (reconfirmed against
// `sbx secret set --help`: positional SANDBOX then SERVICE, with -t/--token
// carrying the secret value).
//
// Dune prefers service-identifier secrets over custom secrets (sbx-4 D5). No
// core boot path sets a service secret in v1 — this is a forward-looking,
// tested surface, not wired into up/Ensure.
func (b *backend) SetServiceSecret(ctx context.Context, spec Spec, service, token string) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}
	if err := validateSecretService(service); err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		return errors.New("secret token is required")
	}
	if _, err := b.runner.Capture(ctx, "", "sbx", "secret", "set", spec.InstanceName, service, "-t", token); err != nil {
		return fmt.Errorf("set sbx service secret for sandbox %q service %q: %w", spec.InstanceName, service, err)
	}
	return nil
}

// ListServiceSecrets lists the stored secrets scoped to the instance's sandbox,
// constructing the verified spike-3 shape `sbx secret ls <instance>` through the
// sbx-3 runner seam (reconfirmed against `sbx secret ls --help`: positional
// SANDBOX). `sbx secret ls` prints a masked, human-readable list; the raw output
// is returned for the caller to surface verbatim.
func (b *backend) ListServiceSecrets(ctx context.Context, spec Spec) ([]byte, error) {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return nil, err
	}
	output, err := b.runner.Capture(ctx, "", "sbx", "secret", "ls", spec.InstanceName)
	if err != nil {
		return output, fmt.Errorf("list sbx secrets for sandbox %q: %w", spec.InstanceName, err)
	}
	return output, nil
}

// RemoveServiceSecret removes a service-identifier secret scoped to the
// instance's sandbox, constructing the verified spike-3 shape `sbx secret rm
// <instance> <service> -f` through the sbx-3 runner seam (reconfirmed against
// `sbx secret rm --help`: positional SANDBOX then SERVICE, with -f/--force
// skipping the confirmation prompt).
func (b *backend) RemoveServiceSecret(ctx context.Context, spec Spec, service string) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}
	if err := validateSecretService(service); err != nil {
		return err
	}
	if _, err := b.runner.Capture(ctx, "", "sbx", "secret", "rm", spec.InstanceName, service, "-f"); err != nil {
		return fmt.Errorf("remove sbx service secret for sandbox %q service %q: %w", spec.InstanceName, service, err)
	}
	return nil
}

// validateSecretService guards the SERVICE positional. The verified spike-3
// shape passes a bare service-identifier token (e.g. "github", "openai"); it
// must not be empty and must not carry shell/flag metacharacters that could
// reshape the constructed sbx invocation.
func validateSecretService(service string) error {
	service = strings.TrimSpace(service)
	if service == "" {
		return errors.New("secret service is required")
	}
	if strings.ContainsAny(service, " \t\r\n") {
		return fmt.Errorf("secret service must be a single token: %q", service)
	}
	if strings.HasPrefix(service, "-") {
		return fmt.Errorf("secret service must not look like a flag: %q", service)
	}
	return nil
}
