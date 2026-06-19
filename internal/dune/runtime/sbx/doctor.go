package sbx

import (
	"context"
	"fmt"
)

const (
	CheckStatusPass = "pass"
	CheckStatusWarn = "warn"
	CheckStatusFail = "fail"
	CheckStatusSkip = "skip"
)

type Check struct {
	ID       string   `json:"id"`
	Group    string   `json:"group"`
	Severity string   `json:"severity"`
	Status   string   `json:"status"`
	Summary  string   `json:"summary"`
	Detail   string   `json:"detail,omitempty"`
	Recovery []string `json:"recovery,omitempty"`
}

type DoctorOptions struct{}

func (b *backend) Doctor(ctx context.Context, spec Spec, _ DoctorOptions) []Check {
	var checks []Check

	if _, err := b.lookPath("sbx"); err != nil {
		checks = append(checks, diagnosticCheck("sbx.path", "host/sbx", "critical", CheckStatusFail, CodeSbxNotInstalled, "sbx is not on PATH", err.Error()))
		checks = append(checks,
			check("sbx.diagnose", "host/sbx", "critical", CheckStatusSkip, "sbx diagnose skipped", "sbx is not available on PATH", nil),
			check("sbx.version", "host/sbx", "critical", CheckStatusSkip, "sbx version skipped", "sbx is not available on PATH", nil),
			templateCheck(spec),
			check("sandbox.status", "sandbox", "info", CheckStatusSkip, "Sandbox status skipped", "sbx is not available on PATH", nil),
			check("egress.posture", "egress", "critical", CheckStatusSkip, "Egress posture skipped", "sbx is not available on PATH", nil),
		)
		return checks
	}
	checks = append(checks, check("sbx.path", "host/sbx", "critical", CheckStatusPass, "sbx is on PATH", "", nil))

	checks = append(checks, b.doctorDiagnoseCheck(ctx))
	checks = append(checks, b.doctorVersionCheck(ctx))
	checks = append(checks, templateCheck(spec))

	state, statusCheck := b.doctorSandboxStatusCheck(ctx, spec)
	checks = append(checks, statusCheck)
	checks = append(checks, b.doctorEgressCheck(ctx, spec, state))
	return checks
}

func (b *backend) doctorDiagnoseCheck(ctx context.Context) Check {
	args := []string{"diagnose", "--output", "json"}
	output, runErr := b.runner.Capture(ctx, "", "sbx", args...)
	report, parseErr := parseDiagnose(output)
	if parseErr != nil {
		detail := parseErr.Error()
		if runErr != nil {
			detail = runErr.Error()
			if trimmed := singleLine(string(output)); trimmed != "" {
				detail += ": " + trimmed
			}
		}
		return diagnosticCheck("sbx.diagnose", "host/sbx", "critical", CheckStatusFail, CodeSbxDiagnoseFailed, "sbx diagnose failed", detail)
	}
	if failed := nonPassingChecks(report); len(failed) > 0 {
		var detail string
		for i, failedCheck := range failed {
			if i > 0 {
				detail += "; "
			}
			detail += fmt.Sprintf("%s=%s", failedCheck.Name, failedCheck.Status)
			if failedCheck.Message != "" {
				detail += " (" + failedCheck.Message + ")"
			}
		}
		return diagnosticCheck("sbx.diagnose", "host/sbx", "critical", CheckStatusFail, CodeSbxDiagnoseFailed, "sbx diagnose reported non-passing checks", detail)
	}
	return check("sbx.diagnose", "host/sbx", "critical", CheckStatusPass, "sbx diagnose passed", "", nil)
}

func (b *backend) doctorVersionCheck(ctx context.Context) Check {
	args := []string{"version"}
	output, err := b.runner.Capture(ctx, "", "sbx", args...)
	if err != nil {
		return diagnosticCheck("sbx.version", "host/sbx", "critical", CheckStatusFail, CodeSbxDiagnoseFailed, "sbx version failed", singleLineDetail(string(output), err.Error()))
	}
	installed, err := parseSbxVersion(output)
	if err != nil {
		return diagnosticCheck("sbx.version", "host/sbx", "critical", CheckStatusFail, CodeSbxDiagnoseFailed, "sbx version returned unreadable output", err.Error())
	}
	ok, err := versionAtLeast(installed, minimumSbxVersion)
	if err != nil {
		return diagnosticCheck("sbx.version", "host/sbx", "critical", CheckStatusFail, CodeSbxDiagnoseFailed, "sbx version could not be compared", err.Error())
	}
	if !ok {
		return diagnosticCheck("sbx.version", "host/sbx", "critical", CheckStatusFail, CodeSbxVersionBelowMin, fmt.Sprintf("sbx %s is older than required %s", installed, minimumSbxVersion), "")
	}
	return check("sbx.version", "host/sbx", "critical", CheckStatusPass, fmt.Sprintf("sbx %s meets minimum %s", installed, minimumSbxVersion), "", nil)
}

func templateCheck(spec Spec) Check {
	if spec.TemplateRef == "" {
		return diagnosticCheck("template.ref", "template", "critical", CheckStatusFail, CodeTemplateUnavailable, "Dune sbx template ref is not configured", "")
	}
	return check("template.ref", "template", "critical", CheckStatusPass, "Dune sbx template ref is configured", spec.TemplateRef, nil)
}

func (b *backend) doctorSandboxStatusCheck(ctx context.Context, spec Spec) (State, Check) {
	state, err := b.Status(ctx, spec)
	if err != nil {
		return State{}, diagnosticCheck("sandbox.status", "sandbox", "warning", CheckStatusWarn, CodeSbxDiagnoseFailed, "Sandbox status could not be inspected", err.Error())
	}
	switch {
	case !state.Exists:
		return state, check("sandbox.status", "sandbox", "info", CheckStatusPass, "Sandbox is absent", "dune doctor does not create sandboxes", nil)
	case state.Running:
		return state, check("sandbox.status", "sandbox", "info", CheckStatusPass, "Sandbox is running", spec.InstanceName, nil)
	default:
		return state, check("sandbox.status", "sandbox", "info", CheckStatusPass, "Sandbox is stopped", spec.InstanceName, nil)
	}
}

func (b *backend) doctorEgressCheck(ctx context.Context, spec Spec, state State) Check {
	if !state.Exists {
		return check("egress.posture", "egress", "critical", CheckStatusSkip, "Egress posture skipped", "sandbox is absent", nil)
	}
	observation := b.inspectEgressPosture(ctx, spec.InstanceName)
	switch observation.posture {
	case egressPostureNonOpen:
		return check("egress.posture", "egress", "critical", CheckStatusPass, "Egress posture is non-Open", observation.detail, nil)
	case egressPostureOpen:
		return check("egress.posture", "egress", "critical", CheckStatusFail, "Egress posture is Open", observation.detail, []string{"Set a non-Open sbx policy default and recreate the sandbox before using Dune."})
	default:
		return check("egress.posture", "egress", "critical", CheckStatusWarn, "Egress posture could not be confirmed", observation.detail, []string{"Inspect with `sbx policy ls` and confirm the sandbox is not using an Open egress posture."})
	}
}

func diagnosticCheck(id, group, severity, status, code, summary, detail string) Check {
	recovery := recoveryHints(code)
	if code != "" {
		summary = code + ": " + summary
	}
	return check(id, group, severity, status, summary, detail, recovery)
}

func check(id, group, severity, status, summary, detail string, recovery []string) Check {
	return Check{
		ID:       id,
		Group:    group,
		Severity: severity,
		Status:   status,
		Summary:  summary,
		Detail:   detail,
		Recovery: append([]string(nil), recovery...),
	}
}
