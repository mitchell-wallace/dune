package dune

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"claudebox/internal/dune/cli"
	sbxruntime "claudebox/internal/dune/runtime/sbx"
	"claudebox/internal/dune/workspace"
	"claudebox/internal/version"
)

const (
	defaultProfile        = "default"
	defaultShell          = "zsh"
	defaultPolicyLogLimit = 50
	helpText              = `Usage: dunex [command] [options]

Commands:
  up               Start or attach to the sandbox (default)
  down             Stop the sandbox
  destroy          Remove the sandbox; persisted profile state is kept
  rebuild          Recreate the sandbox from the Dune sbx template
  logs [service]   Show Dune runtime logs and sbx policy records (default: all)
  ports            List published host ports (default); --publish/--unpublish map them
  doctor           Report host, sandbox, profile, and egress readiness
  version          Print dunex version
  profile set      Set the active profile for the current workspace
  profile list     List stored profile mappings

Global flags:
  -v, --version    Print dunex version and exit
  -h, --help       Show this help message and exit
  -u, --update     Update the dunex CLI to the latest release

Runtime flags (for up/down/destroy/rebuild/logs/ports):
  -d, --directory  Workspace directory (default: current directory)
  -p, --profile    Profile name (default: default)
  -f, --force      Skip destroy confirmation
  --verbose         Show diagnostic command and stderr details on failure
  --publish <spec> Publish a host->sandbox port (dunex ports, repeatable)
  --unpublish <spec>
                   Unpublish a host->sandbox port (dunex ports, repeatable)

Doctor flags:
  --json           Emit structured doctor checks
`
)

var (
	profileNameRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

var newRuntimeBackend = func() sbxruntime.Backend {
	return sbxruntime.NewBackend()
}

type Environment struct {
	CallerPWD string
}

type profileStore map[string]string

func Run(ctx context.Context, argv []string, env Environment, stdin io.Reader, stdout, stderr io.Writer) error {
	opts, err := cli.Parse(argv)
	if err != nil {
		return err
	}

	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if stdin == nil {
		stdin = strings.NewReader("")
	}

	switch opts.Command {
	case cli.CommandVersion:
		_, err := fmt.Fprintf(stdout, "dunex %s\n", version.String())
		return err
	case cli.CommandHelp:
		_, err := fmt.Fprint(stdout, helpText)
		return err
	case cli.CommandUpdate:
		return selfUpdate(ctx, stdout, stderr)
	}
	if opts.Command == cli.CommandDoctor {
		return runDoctor(ctx, opts, env, stdout)
	}

	workspaceInput := defaultWorkspaceInput(opts.WorkspaceInput, env.CallerPWD)
	ws, err := workspace.Resolve(workspaceInput)
	if err != nil {
		return sbxruntime.NewDiagnosticError(sbxruntime.CodeWorkspaceInvalid, "workspace is invalid", err.Error(), err)
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve config directory: %w", err)
	}

	storePath := filepath.Join(configDir, "dune", "profiles.json")
	store, err := loadProfileStore(storePath)
	if err != nil {
		return sbxruntime.NewDiagnosticError(sbxruntime.CodeProfileConfigCorrupt, "profile configuration is corrupt", err.Error(), err)
	}

	switch opts.Command {
	case cli.CommandProfileSet:
		if err := validateProfileName(opts.SetProfileName); err != nil {
			return err
		}
		store[ws.Root] = opts.SetProfileName
		if err := saveProfileStore(storePath, store); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "%s -> %s\n", ws.Root, opts.SetProfileName)
		return nil
	case cli.CommandProfileList:
		return printProfileList(stdout, ws.Root, opts.Profile, opts.ProfileExplicit, store)
	}

	profile, err := resolveProfile(opts, ws.Root, store)
	if err != nil {
		return err
	}

	spec := buildRuntimeSpec(ws, profile)
	return dispatchRuntimeCommand(ctx, opts, spec, newRuntimeBackend(), stdin, stdout, stderr)
}

func RenderError(w io.Writer, err error, verbose bool) {
	if err == nil {
		return
	}
	if w == nil {
		w = io.Discard
	}
	diag, ok := sbxruntime.AsDiagnostic(err)
	if !ok {
		_, _ = fmt.Fprintln(w, err)
		return
	}

	if diag.Code != "" {
		_, _ = fmt.Fprintf(w, "%s: %s\n", diag.Code, diag.Summary)
	} else {
		_, _ = fmt.Fprintln(w, diag.Summary)
	}
	for _, hint := range diag.Recovery {
		if strings.TrimSpace(hint) != "" {
			_, _ = fmt.Fprintf(w, "Recovery: %s\n", hint)
		}
	}
	if !verbose {
		return
	}
	if diag.Detail != "" {
		_, _ = fmt.Fprintf(w, "Detail: %s\n", diag.Detail)
	}
	if len(diag.Command) > 0 {
		_, _ = fmt.Fprintf(w, "Command: %s\n", strings.Join(diag.Command, " "))
	}
	if strings.TrimSpace(diag.Stderr) != "" {
		_, _ = fmt.Fprintf(w, "Stderr:\n%s\n", strings.TrimRight(diag.Stderr, "\n"))
	}
	if diag.Cause != nil {
		_, _ = fmt.Fprintf(w, "Cause: %v\n", diag.Cause)
	}
}

func VerboseRequested(argv []string) bool {
	for _, arg := range argv {
		if arg == "--verbose" {
			return true
		}
	}
	return false
}

func buildRuntimeSpec(ws workspace.Ref, profile string) sbxruntime.Spec {
	return sbxruntime.Spec{
		InstanceName:      sbxruntime.InstanceName(ws.Slug, profile),
		WorkspaceHostPath: ws.Root,
		Profile:           profile,
		TemplateRef:       version.SbxTemplateRef(),
		WorkingDir:        ws.Root,
		Shell:             defaultShell,
		Timezone:          effectiveTimezone(),
	}
}

func dispatchRuntimeCommand(ctx context.Context, opts cli.Options, spec sbxruntime.Spec, backend sbxruntime.Backend, stdin io.Reader, stdout, stderr io.Writer) error {
	streams := sbxruntime.StdIO{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	switch opts.Command {
	case cli.CommandUp:
		return runUp(ctx, backend, spec, streams)
	case cli.CommandDown:
		if err := backend.Validate(ctx); err != nil {
			return err
		}
		return backend.Stop(ctx, spec)
	case cli.CommandDestroy:
		if !opts.Force {
			if err := confirmDestroy(stdin, stderr, spec.InstanceName); err != nil {
				return err
			}
		}
		if err := backend.Validate(ctx); err != nil {
			return err
		}
		return backend.Destroy(ctx, spec)
	case cli.CommandRebuild:
		if err := backend.Validate(ctx); err != nil {
			return err
		}
		return backend.Rebuild(ctx, spec, streams)
	case cli.CommandLogs:
		if err := backend.Validate(ctx); err != nil {
			return err
		}
		return runLogs(ctx, opts, spec, backend, streams)
	case cli.CommandPorts:
		if err := backend.Validate(ctx); err != nil {
			return err
		}
		return runPorts(ctx, opts, spec, backend, streams)
	default:
		return fmt.Errorf("unsupported command %q", opts.Command)
	}
}

func runLogs(ctx context.Context, opts cli.Options, spec sbxruntime.Spec, backend sbxruntime.Backend, streams sbxruntime.StdIO) error {
	if err := backend.Logs(ctx, spec, opts.LogService, streams); err != nil {
		return err
	}
	report, err := backend.PolicyLog(ctx, spec, defaultPolicyLogLimit)
	if err != nil {
		return err
	}
	return writePolicyLogReport(streams.Stdout, report)
}

func writePolicyLogReport(w io.Writer, report sbxruntime.PolicyLogReport) error {
	if w == nil {
		w = io.Discard
	}
	if _, err := fmt.Fprint(w, "\n== sbx policy log ==\n"); err != nil {
		return fmt.Errorf("write policy log heading: %w", err)
	}
	if len(report.BlockedHosts) == 0 && len(report.AllowedHosts) == 0 {
		_, err := fmt.Fprintln(w, "No sbx policy records found.")
		return err
	}
	if err := writePolicyLogRecords(w, "blocked", report.BlockedHosts); err != nil {
		return err
	}
	return writePolicyLogRecords(w, "allowed", report.AllowedHosts)
}

func writePolicyLogRecords(w io.Writer, decision string, records []sbxruntime.PolicyLogRecord) error {
	for _, record := range records {
		if _, err := fmt.Fprintf(w, "%s host=%s rule=%s reason=%s proxy=%s count=%d last_seen=%s\n", decision, record.Host, record.Rule, record.Reason, record.ProxyType, record.CountSince, record.LastSeen); err != nil {
			return fmt.Errorf("write %s policy log record: %w", decision, err)
		}
	}
	return nil
}

// runPorts dispatches the dune ports surface: list is the default; --publish
// and --unpublish map host->sandbox ports through the verified sbx shapes. On a
// publish attempt it surfaces the loopback-vs-all-interfaces caveat (spike 2):
// a nested service bound only to the sandbox loopback may not be reachable via a
// published host port, so dev servers should bind to all sandbox interfaces
// (e.g. --host 0.0.0.0) when host exposure is wanted.
func runPorts(ctx context.Context, opts cli.Options, spec sbxruntime.Spec, backend sbxruntime.Backend, streams sbxruntime.StdIO) error {
	if len(opts.PortsPublish) == 0 && len(opts.PortsUnpublish) == 0 {
		output, err := backend.ListPorts(ctx, spec)
		if err != nil {
			return err
		}
		if _, err := streams.Stdout.Write(output); err != nil {
			return fmt.Errorf("write ports list: %w", err)
		}
		return nil
	}

	if len(opts.PortsPublish) > 0 {
		writeLoopbackBindGuidance(streams.Stderr, spec.InstanceName)
		if err := backend.PublishPorts(ctx, spec, opts.PortsPublish); err != nil {
			return err
		}
	}
	if len(opts.PortsUnpublish) > 0 {
		if err := backend.UnpublishPorts(ctx, spec, opts.PortsUnpublish); err != nil {
			return err
		}
	}
	return nil
}

func writeLoopbackBindGuidance(w io.Writer, instanceName string) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "NOTE: a published host port forwards to the sandbox interface only. If the service inside %q binds solely to sandbox loopback (127.0.0.1), it may not be reachable via the published host port; bind dev servers to all sandbox interfaces (e.g. --host 0.0.0.0) when you want host exposure.\n", instanceName)
}

func confirmDestroy(stdin io.Reader, stderr io.Writer, instanceName string) error {
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	if stderr == nil {
		stderr = io.Discard
	}

	_, _ = fmt.Fprintf(stderr, "This removes sandbox %q. Profile-scoped persisted state is kept.\n", instanceName)
	_, _ = fmt.Fprintf(stderr, "Type %s to confirm: ", instanceName)

	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read destroy confirmation: %w", err)
		}
		return errors.New("destroy cancelled")
	}
	if strings.TrimSpace(scanner.Text()) != instanceName {
		return errors.New("destroy cancelled")
	}
	return nil
}

func runUp(ctx context.Context, backend sbxruntime.Backend, spec sbxruntime.Spec, streams sbxruntime.StdIO) error {
	if err := backend.Validate(ctx); err != nil {
		return err
	}
	// Template availability is realised by Ensure/create against spec.TemplateRef.
	if err := backend.Ensure(ctx, spec); err != nil {
		return err
	}
	if err := backend.VerifyEgressPosture(ctx, spec, streams); err != nil {
		return err
	}
	state, err := backend.Status(ctx, spec)
	if err != nil {
		return err
	}
	if !state.Running {
		if err := backend.Start(ctx, spec); err != nil {
			return err
		}
	}
	return backend.Shell(ctx, spec, streams)
}

type doctorReport struct {
	Status string             `json:"status"`
	Checks []sbxruntime.Check `json:"checks"`
}

func runDoctor(ctx context.Context, opts cli.Options, env Environment, stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}

	checks, ws, profile, runnable := localDoctorChecks(opts, env)
	if runnable {
		spec := buildRuntimeSpec(ws, profile)
		checks = append(checks, newRuntimeBackend().Doctor(ctx, spec, sbxruntime.DoctorOptions{})...)
	} else {
		checks = append(checks,
			doctorCheck("sbx.readiness", "host/sbx", "critical", sbxruntime.CheckStatusSkip, "sbx readiness skipped", "workspace/profile readiness did not resolve", nil),
			doctorCheck("sandbox.status", "sandbox", "info", sbxruntime.CheckStatusSkip, "Sandbox status skipped", "workspace/profile readiness did not resolve", nil),
			doctorCheck("egress.posture", "egress", "critical", sbxruntime.CheckStatusSkip, "Egress posture skipped", "workspace/profile readiness did not resolve", nil),
		)
	}

	report := doctorReport{Status: aggregateDoctorStatus(checks), Checks: checks}
	if opts.JSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	return writeDoctorHuman(stdout, report, opts.Verbose)
}

func localDoctorChecks(opts cli.Options, env Environment) ([]sbxruntime.Check, workspace.Ref, string, bool) {
	var checks []sbxruntime.Check
	runnable := true

	workspaceInput := defaultWorkspaceInput(opts.WorkspaceInput, env.CallerPWD)
	ws, err := workspace.Resolve(workspaceInput)
	if err != nil {
		checks = append(checks, doctorDiagnosticCheck("workspace.resolve", "workspace/profile/config", "critical", sbxruntime.CheckStatusFail, sbxruntime.CodeWorkspaceInvalid, "Workspace could not be resolved", err.Error()))
		runnable = false
	} else {
		checks = append(checks, doctorCheck("workspace.resolve", "workspace/profile/config", "critical", sbxruntime.CheckStatusPass, "Workspace resolved", ws.Root, nil))
		if ws.Slug == "" {
			checks = append(checks, doctorDiagnosticCheck("workspace.slug", "workspace/profile/config", "critical", sbxruntime.CheckStatusFail, sbxruntime.CodeWorkspaceInvalid, "Workspace slug is empty", ""))
			runnable = false
		} else {
			checks = append(checks, doctorCheck("workspace.slug", "workspace/profile/config", "info", sbxruntime.CheckStatusPass, "Workspace slug computed", ws.Slug, nil))
		}
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		checks = append(checks, doctorCheck("config.dir", "workspace/profile/config", "critical", sbxruntime.CheckStatusFail, "Config directory could not be resolved", err.Error(), nil))
		runnable = false
	} else {
		checks = append(checks, dirDoctorCheck("config.dir", "Config directory is usable", filepath.Join(configDir, "dune")))
	}

	dataDir, err := userDataDir()
	if err != nil {
		checks = append(checks, doctorCheck("data.dir", "workspace/profile/config", "critical", sbxruntime.CheckStatusFail, "Data directory could not be resolved", err.Error(), nil))
		runnable = false
	} else {
		checks = append(checks, dirDoctorCheck("data.dir", "Data directory is usable", filepath.Join(dataDir, "dune")))
	}

	store := profileStore{}
	if configDir != "" {
		storePath := filepath.Join(configDir, "dune", "profiles.json")
		loaded, err := loadProfileStore(storePath)
		if err != nil {
			checks = append(checks, doctorDiagnosticCheck("profile.store", "workspace/profile/config", "critical", sbxruntime.CheckStatusFail, sbxruntime.CodeProfileConfigCorrupt, "Profile mappings could not be parsed", err.Error()))
			runnable = false
		} else {
			store = loaded
			checks = append(checks, doctorCheck("profile.store", "workspace/profile/config", "critical", sbxruntime.CheckStatusPass, "Profile mappings are readable", storePath+" (missing is ok)", nil))
		}
	}

	profile := defaultProfile
	if runnable || ws.Root != "" {
		var err error
		profile, err = resolveProfile(opts, ws.Root, store)
		if err != nil {
			checks = append(checks, doctorCheck("profile.effective", "workspace/profile/config", "critical", sbxruntime.CheckStatusFail, "Effective profile is invalid", err.Error(), nil))
			runnable = false
		} else {
			checks = append(checks, doctorCheck("profile.effective", "workspace/profile/config", "critical", sbxruntime.CheckStatusPass, "Effective profile resolved", profile, nil))
		}
	} else {
		checks = append(checks, doctorCheck("profile.effective", "workspace/profile/config", "critical", sbxruntime.CheckStatusSkip, "Effective profile skipped", "workspace did not resolve", nil))
	}

	if profile != "" {
		persistPath, err := sbxruntime.ProfilePersistHostPath(profile)
		if err != nil {
			checks = append(checks, doctorCheck("persist.dir", "workspace/profile/config", "critical", sbxruntime.CheckStatusFail, "Profile persist directory could not be resolved", err.Error(), nil))
			runnable = false
		} else {
			checks = append(checks, dirDoctorCheck("persist.dir", "Profile persist directory is usable", persistPath))
		}
	}

	return checks, ws, profile, runnable
}

func writeDoctorHuman(w io.Writer, report doctorReport, verbose bool) error {
	if _, err := fmt.Fprintln(w, "Dune doctor"); err != nil {
		return err
	}
	for _, check := range report.Checks {
		if _, err := fmt.Fprintf(w, "%-4s %-24s %s\n", strings.ToUpper(check.Status), check.Group, check.Summary); err != nil {
			return err
		}
		if verbose {
			if check.Detail != "" {
				if _, err := fmt.Fprintf(w, "     detail: %s\n", check.Detail); err != nil {
					return err
				}
			}
			for _, hint := range check.Recovery {
				if _, err := fmt.Fprintf(w, "     recovery: %s\n", hint); err != nil {
					return err
				}
			}
		}
	}
	counts := map[string]int{}
	for _, check := range report.Checks {
		counts[check.Status]++
	}
	_, err := fmt.Fprintf(w, "\n%d failed, %d warning, %d passed, %d skipped\n", counts[sbxruntime.CheckStatusFail], counts[sbxruntime.CheckStatusWarn], counts[sbxruntime.CheckStatusPass], counts[sbxruntime.CheckStatusSkip])
	return err
}

func aggregateDoctorStatus(checks []sbxruntime.Check) string {
	for _, check := range checks {
		if check.Status == sbxruntime.CheckStatusFail {
			return sbxruntime.CheckStatusFail
		}
	}
	for _, check := range checks {
		if check.Status == sbxruntime.CheckStatusWarn {
			return sbxruntime.CheckStatusWarn
		}
	}
	return sbxruntime.CheckStatusPass
}

func dirDoctorCheck(id, summary, path string) sbxruntime.Check {
	if err := pathReadWriteOrCreatable(path); err != nil {
		return doctorCheck(id, "workspace/profile/config", "critical", sbxruntime.CheckStatusFail, summary+" check failed", err.Error(), nil)
	}
	return doctorCheck(id, "workspace/profile/config", "critical", sbxruntime.CheckStatusPass, summary, path, nil)
}

func pathReadWriteOrCreatable(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path is empty")
	}
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s exists but is not a directory", path)
		}
		if err := readableWritableDir(path, info); err != nil {
			return err
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	for parent := filepath.Dir(path); parent != "." && parent != string(filepath.Separator); parent = filepath.Dir(parent) {
		info, err := os.Stat(parent)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			return fmt.Errorf("nearest existing parent %s is not a directory", parent)
		}
		if info.Mode().Perm()&0o222 == 0 {
			return fmt.Errorf("%s does not exist and parent %s is not writable", path, parent)
		}
		return nil
	}
	return fmt.Errorf("%s does not exist and no writable parent was found", path)
}

func readableWritableDir(path string, info os.FileInfo) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%s is not readable: %w", path, err)
	}
	_ = file.Close()
	if info.Mode().Perm()&0o222 == 0 {
		return fmt.Errorf("%s is not writable", path)
	}
	return nil
}

func userDataDir() (string, error) {
	if dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); dataHome != "" {
		return dataHome, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share"), nil
}

func doctorDiagnosticCheck(id, group, severity, status, code, summary, detail string) sbxruntime.Check {
	return doctorCheck(id, group, severity, status, code+": "+summary, detail, doctorRecoveryHints(code))
}

func doctorRecoveryHints(code string) []string {
	switch code {
	case sbxruntime.CodeWorkspaceInvalid:
		return []string{"Run dune from a valid workspace path or pass `--directory` with an existing project directory."}
	case sbxruntime.CodeProfileConfigCorrupt:
		return []string{"Fix or remove the Dune profiles.json file, then retry."}
	default:
		return nil
	}
}

func doctorCheck(id, group, severity, status, summary, detail string, recovery []string) sbxruntime.Check {
	return sbxruntime.Check{
		ID:       id,
		Group:    group,
		Severity: severity,
		Status:   status,
		Summary:  summary,
		Detail:   detail,
		Recovery: append([]string(nil), recovery...),
	}
}

func selfUpdate(ctx context.Context, stdout, stderr io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}

	current := strings.TrimPrefix(version.Version, "v")
	if current == "dev" || current == "" {
		return errors.New("cannot update development build; please reinstall manually")
	}

	_, _ = fmt.Fprintln(stderr, "Checking for updates...")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/mitchell-wallace/dune/releases/latest", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github API returned %s", resp.Status)
	}

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("decode release response: %w", err)
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	if latest == current {
		_, _ = fmt.Fprintf(stdout, "dune %s is already the latest version.\n", current)
		return nil
	}

	assetName := fmt.Sprintf("dune_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	for _, a := range release.Assets {
		if a.Name == assetName {
			downloadURL = a.URL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no release asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	_, _ = fmt.Fprintf(stderr, "Downloading dune %s...\n", latest)

	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}

	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		return fmt.Errorf("download release: %w", err)
	}
	defer func() { _ = dlResp.Body.Close() }()

	if dlResp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", dlResp.Status)
	}

	gr, err := gzip.NewReader(dlResp.Body)
	if err != nil {
		return fmt.Errorf("open gzip archive: %w", err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	var newBinary []byte
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar archive: %w", err)
		}
		if h.Name == "dune" || filepath.Base(h.Name) == "dune" {
			newBinary, err = io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("read binary from archive: %w", err)
			}
			break
		}
	}
	if newBinary == nil {
		return errors.New("dune binary not found in release archive")
	}

	tmpPath := exe + ".tmp"
	if err := os.WriteFile(tmpPath, newBinary, 0755); err != nil {
		return fmt.Errorf("write new binary: %w", err)
	}

	if err := os.Rename(tmpPath, exe); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace binary: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Updated dune %s -> %s\n", current, latest)
	return nil
}

func resolveProfile(opts cli.Options, workspaceRoot string, store profileStore) (string, error) {
	if opts.ProfileExplicit {
		if err := validateProfileName(opts.Profile); err != nil {
			return "", err
		}
		return opts.Profile, nil
	}
	if stored := strings.TrimSpace(store[workspaceRoot]); stored != "" {
		return stored, nil
	}
	return defaultProfile, nil
}

func printProfileList(stdout io.Writer, workspaceRoot, explicit string, explicitSet bool, store profileStore) error {
	keys := make([]string, 0, len(store))
	for key := range store {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	effective := defaultProfile
	if explicitSet {
		effective = explicit
	} else if mapped := store[workspaceRoot]; mapped != "" {
		effective = mapped
	}

	if _, err := fmt.Fprintf(stdout, "Effective profile for %s: %s\n", workspaceRoot, effective); err != nil {
		return err
	}
	if len(keys) == 0 {
		_, err := fmt.Fprintln(stdout, "No stored profile mappings.")
		return err
	}

	for _, key := range keys {
		marker := " "
		if key == workspaceRoot {
			marker = "*"
		}
		if _, err := fmt.Fprintf(stdout, "%s %s -> %s\n", marker, key, store[key]); err != nil {
			return err
		}
	}
	return nil
}

func validateProfileName(name string) error {
	if !profileNameRE.MatchString(name) {
		return fmt.Errorf("invalid profile %q: use lowercase letters, numbers, and hyphens only", name)
	}
	return nil
}

func loadProfileStore(path string) (profileStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return profileStore{}, nil
		}
		return nil, fmt.Errorf("read profile mappings: %w", err)
	}

	var store profileStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parse profile mappings: %w", err)
	}
	if store == nil {
		store = profileStore{}
	}
	return store, nil
}

func saveProfileStore(path string, store profileStore) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create profile mapping directory: %w", err)
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile mappings: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write profile mappings: %w", err)
	}
	return nil
}

func defaultWorkspaceInput(explicit, callerPWD string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if strings.TrimSpace(callerPWD) != "" {
		return callerPWD
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func effectiveTimezone() string {
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" {
		return tz
	}
	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		if tz := strings.TrimSpace(string(data)); tz != "" {
			return tz
		}
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		const prefix = "/usr/share/zoneinfo/"
		if after, ok := strings.CutPrefix(target, prefix); ok {
			if tz := strings.TrimSpace(after); tz != "" {
				return tz
			}
		}
		if i := strings.Index(target, prefix); i >= 0 {
			return target[i+len(prefix):]
		}
	}
	return "UTC"
}
