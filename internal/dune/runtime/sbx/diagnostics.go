package sbx

import (
	"errors"
	"os/exec"
	"strings"
)

const (
	CodeSbxNotInstalled      = "sbx.not_installed"
	CodeSbxDiagnoseFailed    = "sbx.diagnose_failed"
	CodeSbxVersionBelowMin   = "sbx.version_below_min"
	CodeSbxCreateFailed      = "sbx.create_failed"
	CodeSbxStartFailed       = "sbx.start_failed"
	CodeSbxStopFailed        = "sbx.stop_failed"
	CodeSbxExecFailed        = "sbx.exec_failed"
	CodeSbxRmFailed          = "sbx.rm_failed"
	CodeTemplateUnavailable  = "template.unavailable"
	CodePolicyApplyFailed    = "policy.apply_failed"
	CodeWorkspaceInvalid     = "workspace.invalid"
	CodeProfileConfigCorrupt = "profile.config_corrupt"
)

type DiagnosticError struct {
	Code     string
	Summary  string
	Detail   string
	Cause    error
	Command  []string
	Stderr   string
	Recovery []string
}

func (e *DiagnosticError) Error() string {
	if e == nil {
		return ""
	}
	var parts []string
	if e.Summary != "" {
		parts = append(parts, e.Summary)
	}
	if e.Detail != "" {
		parts = append(parts, e.Detail)
	}
	if e.Cause != nil {
		parts = append(parts, e.Cause.Error())
	}
	if len(parts) > 0 {
		return strings.Join(parts, ": ")
	}
	if e.Code != "" {
		return e.Code
	}
	return "diagnostic error"
}

func (e *DiagnosticError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func IsDiagnostic(err error) bool {
	_, ok := AsDiagnostic(err)
	return ok
}

func AsDiagnostic(err error) (*DiagnosticError, bool) {
	var diag *DiagnosticError
	if errors.As(err, &diag) {
		return diag, true
	}
	return nil, false
}

type CommandResult struct {
	Command []string
	Stderr  string
}

func WrapCommandError(code, summary string, result CommandResult, err error) error {
	if err == nil {
		return nil
	}
	return &DiagnosticError{
		Code:     code,
		Summary:  summary,
		Cause:    err,
		Command:  append([]string(nil), result.Command...),
		Stderr:   result.Stderr,
		Recovery: recoveryHints(code),
	}
}

func NewDiagnosticError(code, summary, detail string, cause error) error {
	return &DiagnosticError{
		Code:     code,
		Summary:  summary,
		Detail:   detail,
		Cause:    cause,
		Recovery: recoveryHints(code),
	}
}

func commandResult(name string, args []string, output []byte, err error) CommandResult {
	result := CommandResult{Command: append([]string{name}, args...)}
	if len(output) > 0 {
		result.Stderr = string(output)
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		result.Stderr = string(exitErr.Stderr)
	}
	return result
}

func recoveryHints(code string) []string {
	switch code {
	case CodeSbxNotInstalled:
		return []string{"Install sbx, ensure it is on PATH, then retry."}
	case CodeSbxDiagnoseFailed:
		return []string{"Run `sbx diagnose --output json` and resolve any failing checks.", "If authentication failed, run `sbx login`."}
	case CodeSbxVersionBelowMin:
		return []string{"upgrade sbx to the minimum supported version and retry."}
	case CodeTemplateUnavailable:
		return []string{"Confirm the Dune sbx template reference is reachable from this host, then retry."}
	case CodePolicyApplyFailed:
		return []string{"Inspect the sbx policy command output, fix the policy rule or sandbox state, then retry."}
	case CodeWorkspaceInvalid:
		return []string{"Run dune from a valid workspace path or pass `--directory` with an existing project directory."}
	case CodeProfileConfigCorrupt:
		return []string{"Fix or remove the Dune profiles.json file, then retry."}
	default:
		return nil
	}
}

func singleLineDetail(parts ...string) string {
	var kept []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "; ")
}
