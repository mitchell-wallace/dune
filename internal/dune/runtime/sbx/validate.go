package sbx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// minimumSbxVersion is the lowest sbx CLI version Dune supports. The sbx
// command shapes this backend pins (sbx diagnose --output json, sbx version,
// and the lifecycle shapes in later laps) were verified against v0.32.0 by the
// sbx-1 spikes; older sbx releases are rejected rather than guessed at.
const minimumSbxVersion = "v0.32.0"

// Validate confirms the host is ready to drive sandboxes. It checks, in order:
// sbx is on PATH; `sbx diagnose --output json` reports every check passing
// (this also covers daemon health and authentication — the spikes observed 8
// checks pass in a healthy install); and the installed sbx version meets
// minimumSbxVersion. Each unmet requirement returns a clear, actionable error
// and no further sandbox operation is attempted.
func (b *backend) Validate(ctx context.Context) error {
	if _, err := b.lookPath("sbx"); err != nil {
		return errors.New("sbx is not installed or not on PATH; install sbx and try again")
	}

	diagOutput, diagErr := b.runner.Capture(ctx, "", "sbx", "diagnose", "--output", "json")
	report, parseErr := parseDiagnose(diagOutput)
	if parseErr != nil {
		// diagnose is the source of truth; when its JSON cannot be parsed the
		// underlying run error (if any) is the most useful thing to surface.
		if diagErr != nil {
			return fmt.Errorf("run sbx diagnose: %w", diagErr)
		}
		return fmt.Errorf("parse sbx diagnose output: %w", parseErr)
	}
	if failed := nonPassingChecks(report); len(failed) > 0 {
		return diagnoseError(failed)
	}

	versionOutput, err := b.runner.Capture(ctx, "", "sbx", "version")
	if err != nil {
		return fmt.Errorf("run sbx version: %w", err)
	}
	installed, err := parseSbxVersion(versionOutput)
	if err != nil {
		return fmt.Errorf("read sbx version: %w", err)
	}
	ok, err := versionAtLeast(installed, minimumSbxVersion)
	if err != nil {
		return fmt.Errorf("compare sbx version: %w", err)
	}
	if !ok {
		return fmt.Errorf("sbx %s is older than the required %s; upgrade sbx and try again", installed, minimumSbxVersion)
	}
	return nil
}

// diagnoseReport mirrors the verified `sbx diagnose --output json` shape. Pin
// these field names in fakeRunner tests so a future sbx output change surfaces
// as a failing test.
type diagnoseReport struct {
	Version string          `json:"version"`
	Checks  []diagnoseCheck `json:"checks"`
	Summary diagnoseSummary `json:"summary"`
}

type diagnoseCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Detail  string `json:"detail"`
	Hint    string `json:"hint"`
}

type diagnoseSummary struct {
	Pass int `json:"pass"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
	Skip int `json:"skip"`
}

func parseDiagnose(output []byte) (diagnoseReport, error) {
	var report diagnoseReport
	if err := json.Unmarshal(output, &report); err != nil {
		return report, fmt.Errorf("parse diagnose JSON: %w", err)
	}
	return report, nil
}

// nonPassingChecks returns every check whose status is not "pass". A healthy
// install reports all checks as "pass"; anything else (fail/warn/skip) means
// the host is not ready to drive sandboxes.
func nonPassingChecks(report diagnoseReport) []diagnoseCheck {
	var failed []diagnoseCheck
	for _, check := range report.Checks {
		if check.Status != "pass" {
			failed = append(failed, check)
		}
	}
	return failed
}

func diagnoseError(failed []diagnoseCheck) error {
	var b strings.Builder
	fmt.Fprintf(&b, "sbx is not ready: diagnose reported %d check(s) not passing", len(failed))
	for _, check := range failed {
		b.WriteString("; ")
		b.WriteString(check.Name)
		b.WriteString(" (")
		b.WriteString(check.Status)
		if check.Message != "" {
			b.WriteString(": ")
			b.WriteString(check.Message)
		}
		b.WriteString(")")
		if check.Hint != "" {
			b.WriteString(" -- ")
			b.WriteString(check.Hint)
		}
	}
	return errors.New(b.String())
}

// parseSbxVersion extracts the version token from `sbx version` output.
// Observed shape: "sbx version: v0.32.0 55580366449bcfebfc1787b9944284cf64c856d7".
func parseSbxVersion(output []byte) (string, error) {
	const prefix = "sbx version: "
	line := strings.TrimSpace(string(output))
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("unrecognised sbx version output %q", line)
	}
	fields := strings.Fields(strings.TrimPrefix(line, prefix))
	if len(fields) == 0 {
		return "", fmt.Errorf("sbx version output missing version token: %q", line)
	}
	return fields[0], nil
}

// versionAtLeast reports whether actual >= minimum using a numeric
// major.minor.patch comparison. A leading "v" is optional and pre-release /
// build metadata (anything after '-' or '+') is ignored, so the comparison is
// on the numeric core only. This is deliberately a small semver-style check,
// not a full semver implementation.
func versionAtLeast(actual, minimum string) (bool, error) {
	actualCore, err := parseVersionCore(actual)
	if err != nil {
		return false, fmt.Errorf("actual %q: %w", actual, err)
	}
	minimumCore, err := parseVersionCore(minimum)
	if err != nil {
		return false, fmt.Errorf("minimum %q: %w", minimum, err)
	}
	for i := 0; i < 3; i++ {
		if actualCore[i] != minimumCore[i] {
			return actualCore[i] > minimumCore[i], nil
		}
	}
	return true, nil
}

func parseVersionCore(version string) ([3]int, error) {
	var core [3]int
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return core, errors.New("empty version")
	}
	parts := strings.Split(v, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return core, fmt.Errorf("component %q is not numeric: %w", parts[i], err)
		}
		if n < 0 {
			return core, fmt.Errorf("component %q is negative", parts[i])
		}
		core[i] = n
	}
	return core, nil
}
