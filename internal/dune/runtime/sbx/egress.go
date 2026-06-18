package sbx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

type egressPosture int

const (
	egressPostureUnknown egressPosture = iota
	egressPostureNonOpen
	egressPostureOpen
)

type egressPostureObservation struct {
	posture egressPosture
	detail  string
}

type policyListRow struct {
	PolicyRule string
	Type       string
	Decision   string
	Resources  string
}

var policyColumnSplitter = regexp.MustCompile(`[ \t]{2,}`)

// VerifyEgressPosture inspects the instance's active network policy without
// mutating sbx policy. Unconfirmable posture is a warning so a host with an
// already closed global default is not blocked; positively observed Open
// posture is a hard failure.
func (b *backend) VerifyEgressPosture(ctx context.Context, spec Spec, streams StdIO) error {
	if err := validateInstanceName(spec.InstanceName); err != nil {
		return err
	}

	observation := b.inspectEgressPosture(ctx, spec.InstanceName)
	switch observation.posture {
	case egressPostureNonOpen:
		return nil
	case egressPostureOpen:
		return openEgressPostureError(spec.InstanceName, observation.detail)
	default:
		writeUnconfirmedEgressWarning(streams.Stderr, spec.InstanceName, observation.detail)
		return nil
	}
}

func (b *backend) inspectEgressPosture(ctx context.Context, instanceName string) egressPostureObservation {
	output, err := b.runner.Capture(ctx, "", "sbx", "policy", "ls", instanceName, "--type", "network")
	if err != nil {
		detail := fmt.Sprintf("sbx policy ls %q --type network failed: %v", instanceName, err)
		if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
			detail += ": " + singleLine(trimmed)
		}
		return egressPostureObservation{posture: egressPostureUnknown, detail: detail}
	}

	rows, err := parsePolicyList(output)
	if err != nil {
		return egressPostureObservation{
			posture: egressPostureUnknown,
			detail:  "could not parse sbx policy ls output: " + err.Error(),
		}
	}

	return classifyEgressPosture(rows)
}

func parsePolicyList(output []byte) ([]policyListRow, error) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return nil, errors.New("empty sbx policy ls output")
	}

	lines := strings.Split(string(trimmed), "\n")
	headerIndex := -1
	var indexes policyColumnIndexes
	for i, line := range lines {
		cols := splitPolicyColumns(line)
		if cols == nil {
			continue
		}
		if got, ok := policyColumnIndexesFromHeader(cols); ok {
			headerIndex = i
			indexes = got
			break
		}
	}
	if headerIndex == -1 {
		return nil, errors.New("missing policy table header")
	}

	var rows []policyListRow
	for _, line := range lines[headerIndex+1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cols := splitPolicyColumns(line)
		if len(cols) <= indexes.maxRequired() {
			return nil, fmt.Errorf("malformed policy row %q", line)
		}

		row := policyListRow{
			Type:      cols[indexes.typ],
			Decision:  cols[indexes.decision],
			Resources: cols[indexes.resources],
		}
		if indexes.policyRule >= 0 && len(cols) > indexes.policyRule {
			row.PolicyRule = cols[indexes.policyRule]
		}
		rows = append(rows, row)
	}

	return rows, nil
}

type policyColumnIndexes struct {
	policyRule int
	typ        int
	decision   int
	resources  int
}

func (i policyColumnIndexes) maxRequired() int {
	max := i.typ
	if i.decision > max {
		max = i.decision
	}
	if i.resources > max {
		max = i.resources
	}
	return max
}

func policyColumnIndexesFromHeader(cols []string) (policyColumnIndexes, bool) {
	indexes := policyColumnIndexes{policyRule: -1, typ: -1, decision: -1, resources: -1}
	for i, col := range cols {
		switch normalizePolicyHeader(col) {
		case "policyrule", "rule", "policy":
			indexes.policyRule = i
		case "type":
			indexes.typ = i
		case "decision":
			indexes.decision = i
		case "resources", "resource":
			indexes.resources = i
		}
	}
	return indexes, indexes.typ >= 0 && indexes.decision >= 0 && indexes.resources >= 0
}

func splitPolicyColumns(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	return policyColumnSplitter.Split(line, -1)
}

func normalizePolicyHeader(header string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(header) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func classifyEgressPosture(rows []policyListRow) egressPostureObservation {
	if len(rows) == 0 {
		return egressPostureObservation{
			posture: egressPostureUnknown,
			detail:  "no network policy rows returned by sbx policy ls",
		}
	}

	hasNetwork := false
	hasAllowAll := false
	hasRestrictiveAllow := false
	hasUnknownDecision := false
	var allowAllRule policyListRow

	for _, row := range rows {
		if !strings.EqualFold(strings.TrimSpace(row.Type), "network") {
			continue
		}
		hasNetwork = true
		switch strings.ToLower(strings.TrimSpace(row.Decision)) {
		case "deny":
			return egressPostureObservation{
				posture: egressPostureNonOpen,
				detail:  "observed network deny rule " + quotePolicyRule(row),
			}
		case "allow":
			if policyResourcesAllowAll(row.Resources) {
				hasAllowAll = true
				allowAllRule = row
			} else {
				hasRestrictiveAllow = true
			}
		default:
			hasUnknownDecision = true
		}
	}

	if !hasNetwork {
		return egressPostureObservation{
			posture: egressPostureUnknown,
			detail:  "no network policy rows returned by sbx policy ls",
		}
	}
	if hasUnknownDecision {
		return egressPostureObservation{
			posture: egressPostureUnknown,
			detail:  "network policy rows used an unknown decision value",
		}
	}
	if hasAllowAll {
		return egressPostureObservation{
			posture: egressPostureOpen,
			detail:  fmt.Sprintf("observed allow-all network rule %s with resources %q", quotePolicyRule(allowAllRule), allowAllRule.Resources),
		}
	}
	if hasRestrictiveAllow {
		return egressPostureObservation{
			posture: egressPostureNonOpen,
			detail:  "observed network allowlist without an allow-all rule",
		}
	}

	return egressPostureObservation{
		posture: egressPostureUnknown,
		detail:  "network policy rows did not identify allow or deny decisions",
	}
}

func policyResourcesAllowAll(resources string) bool {
	for _, token := range strings.FieldsFunc(resources, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t'
	}) {
		switch strings.TrimSpace(token) {
		case "**", "*":
			return true
		}
	}
	return false
}

func writeUnconfirmedEgressWarning(w io.Writer, instanceName, detail string) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "WARNING: could not confirm a non-Open sbx egress posture for sandbox %q (%s). Dune will continue without changing sbx policy. Set a non-Open default before creating sandboxes with \"sbx policy set-default balanced\" (recommended) or \"sbx policy set-default deny-all\"; then use \"sbx policy allow network --sandbox %s <domain>:443\" for project-specific domains.\n", instanceName, detail, instanceName)
}

func openEgressPostureError(instanceName, detail string) error {
	return fmt.Errorf("sbx egress posture for sandbox %q is Open (%s); Dune will not operate under Open egress. Set a non-Open default before creating sandboxes with \"sbx policy set-default balanced\" (recommended) or \"sbx policy set-default deny-all\", recreate the sandbox, and retry", instanceName, detail)
}

func quotePolicyRule(row policyListRow) string {
	name := strings.TrimSpace(row.PolicyRule)
	if name == "" {
		return "<unnamed>"
	}
	return fmt.Sprintf("%q", name)
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
