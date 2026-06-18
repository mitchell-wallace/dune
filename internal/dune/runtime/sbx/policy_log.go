package sbx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// PolicyLogReport is the parsed output of `sbx policy log <instance> --json`.
// The top-level shape groups observed hosts by decision: blocked (denied) and
// allowed. Field names are pinned from observed sbx v0.32.0 output so a future
// sbx output change surfaces as a failing fakeRunner test (mirroring the
// diagnose convention in validate.go).
type PolicyLogReport struct {
	BlockedHosts []PolicyLogRecord `json:"blocked_hosts"`
	AllowedHosts []PolicyLogRecord `json:"allowed_hosts"`
}

// PolicyLogRecord is a single observed network policy event. ProxyType
// distinguishes how the traffic reached the sbx proxy: "forward" for direct
// shell traffic, "transparent" for nested-container traffic (e.g. nested
// Docker), and "forward-bypass" for traffic that bypassed the policy engine.
// The parse does not assume a single ProxyType; the spikes recorded both
// forward and transparent records for the same deny.
type PolicyLogRecord struct {
	Host       string `json:"host"`
	VMName     string `json:"vm_name"`
	ProxyType  string `json:"proxy_type"`
	Rule       string `json:"rule"`
	Reason     string `json:"reason"`
	LastSeen   string `json:"last_seen"`
	Since      string `json:"since"`
	CountSince int    `json:"count_since"`
}

// PolicyLog returns the recorded network policy events for the instance's
// sandbox, wrapping `sbx policy log <instance> --json --limit <limit>` through
// the sbx-3 runner seam. It is the sbx egress observability source; the final
// `dune logs` composition is sbx-5.
//
// limit caps the number of records sbx returns and is forwarded as --limit; it
// must be greater than zero. The structured --json form is used so the parsed
// field names can be pinned in fakeRunner tests.
func (b *backend) PolicyLog(ctx context.Context, spec Spec, limit int) (PolicyLogReport, error) {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return PolicyLogReport{}, err
	}
	if err := validatePolicyLogLimit(limit); err != nil {
		return PolicyLogReport{}, err
	}

	output, err := b.runner.Capture(ctx, "", "sbx", "policy", "log", spec.InstanceName, "--json", "--limit", strconv.Itoa(limit))
	if err != nil {
		return PolicyLogReport{}, fmt.Errorf("read sbx policy log for sandbox %q: %w", spec.InstanceName, err)
	}

	report, err := parsePolicyLog(output)
	if err != nil {
		return PolicyLogReport{}, fmt.Errorf("parse sbx policy log output for sandbox %q: %w", spec.InstanceName, err)
	}
	return report, nil
}

func validatePolicyLogLimit(limit int) error {
	if limit <= 0 {
		return errors.New("policy log limit must be greater than zero")
	}
	return nil
}

// parsePolicyLog decodes the verified `sbx policy log <instance> --json` shape
// (an object with "blocked_hosts" and "allowed_hosts" arrays). It mirrors the
// JSON-shape-tolerant style of parseSandboxList: leading/trailing whitespace is
// trimmed, an empty payload is rejected, and an unexpected leading token yields
// a clear error. A missing decision array is treated as empty rather than an
// error, so a sandbox with no observed traffic parses as a zero-length report.
func parsePolicyLog(output []byte) (PolicyLogReport, error) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return PolicyLogReport{}, errors.New("empty sbx policy log output")
	}
	if trimmed[0] != '{' {
		return PolicyLogReport{}, fmt.Errorf("unexpected sbx policy log JSON: expected object, got %q", trimmed[0])
	}

	var report PolicyLogReport
	if err := json.Unmarshal(trimmed, &report); err != nil {
		return PolicyLogReport{}, fmt.Errorf("parse policy log JSON: %w", err)
	}
	return report, nil
}
