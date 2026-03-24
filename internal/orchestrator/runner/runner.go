package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"claudebox/internal/orchestrator/contract"
	"claudebox/internal/orchestrator/messages"
	"claudebox/internal/orchestrator/progress"
	"claudebox/internal/orchestrator/state"
)

type AgentMix struct {
	Weights map[string]int
	Order   []string
	Cycle   []string
	Label   string
}

type Config struct {
	WorkspaceDir     string
	DataDir          string
	RepoProgressPath string
	AgentSpecs       []string
	Iterations       int
	Stdout           io.Writer
	Stderr           io.Writer
}

type Runner struct {
	cfg          Config
	stateStore   *state.Store
	messageStore *messages.Store
}

type SessionResult struct {
	SessionID      int
	BatchID        int
	IterationIndex int
	Agent          string
	ExitCode       int
}

func New(cfg Config) *Runner {
	return &Runner{
		cfg:          cfg,
		stateStore:   state.NewStore(cfg.DataDir),
		messageStore: messages.NewStore(cfg.DataDir),
	}
}

func ParseAgentMix(specs []string) (AgentMix, error) {
	weights := map[string]int{"claude": 0, "codex": 0, "gemini": 0, "opencode": 0}
	order := []string{}
	addWeight := func(agent string, amount int) error {
		if amount < 1 {
			return fmt.Errorf("agent weight must be >= 1")
		}
		if weights[agent] == 0 {
			order = append(order, agent)
		}
		weights[agent] += amount
		return nil
	}

	if len(specs) == 0 {
		_ = addWeight("claude", 1)
		_ = addWeight("codex", 2)
	} else {
		aliases := map[string]string{
			"cc": "claude", "claude": "claude",
			"cx": "codex", "codex": "codex",
			"ge": "gemini", "gemini": "gemini",
			"op": "opencode", "opencode": "opencode",
		}
		for _, spec := range specs {
			parts := strings.SplitN(spec, ":", 2)
			agent, ok := aliases[parts[0]]
			if !ok {
				return AgentMix{}, fmt.Errorf("unknown agent alias %q", parts[0])
			}
			weight := 1
			if len(parts) == 2 {
				n, err := strconv.Atoi(parts[1])
				if err != nil || n < 1 {
					return AgentMix{}, fmt.Errorf("invalid agent weight %q", spec)
				}
				weight = n
			}
			if err := addWeight(agent, weight); err != nil {
				return AgentMix{}, err
			}
		}
	}

	cycle := []string{}
	labelParts := []string{}
	for _, agent := range order {
		for i := 0; i < weights[agent]; i++ {
			cycle = append(cycle, agent)
		}
		labelParts = append(labelParts, fmt.Sprintf("%s:%d", agent, weights[agent]))
	}
	return AgentMix{
		Weights: weights,
		Order:   order,
		Cycle:   cycle,
		Label:   strings.Join(labelParts, " "),
	}, nil
}

func AgentForSession(sessionID int, mix AgentMix) string {
	if len(mix.Cycle) == 0 {
		return "claude"
	}
	return mix.Cycle[(sessionID-1)%len(mix.Cycle)]
}

func BuildAgentCommand(agentName, prompt string) ([]string, bool, error) {
	switch agentName {
	case "claude":
		return []string{"claude", "-p", "--dangerously-skip-permissions", "--output-format", "text", prompt}, false, nil
	case "codex":
		return []string{"codex", "exec", "--dangerously-bypass-approvals-and-sandbox", prompt}, true, nil
	case "gemini":
		return []string{"gemini", "--prompt", prompt, "--yolo", "--output-format", "text"}, false, nil
	case "opencode":
		return []string{"opencode", "run", prompt}, false, nil
	default:
		return nil, false, fmt.Errorf("unsupported agent %q", agentName)
	}
}

func (r *Runner) EnsureInitialized() error {
	st, err := r.stateStore.Load()
	if err != nil {
		return err
	}
	return r.stateStore.Save(st)
}

func (r *Runner) StartOrResumeBatch(iterations int) (state.State, error) {
	st, err := r.stateStore.Load()
	if err != nil {
		return state.State{}, err
	}
	mix, err := ParseAgentMix(r.cfg.AgentSpecs)
	if err != nil {
		return state.State{}, err
	}
	if st.ActiveBatch == nil {
		st.ActiveBatch = &state.BatchState{
			BatchID:          st.NextBatchID,
			TargetIterations: iterations,
			AgentMix:         append([]string{}, r.cfg.AgentSpecs...),
			StartedAt:        time.Now().UTC().Format(time.RFC3339),
		}
		st.NextBatchID++
	} else {
		if iterations > 0 && iterations < st.ActiveBatch.CompletedIterations {
			iterations = st.ActiveBatch.CompletedIterations
		}
		if iterations > 0 {
			st.ActiveBatch.TargetIterations = iterations
		}
	}
	if len(st.ActiveBatch.AgentMix) == 0 {
		st.ActiveBatch.AgentMix = mix.Order
	}
	if err := r.stateStore.Save(st); err != nil {
		return state.State{}, err
	}
	return st, nil
}

func (r *Runner) RequestStopAfterCurrent() error {
	st, err := r.stateStore.Load()
	if err != nil {
		return err
	}
	st.StopAfterCurrent = true
	return r.stateStore.Save(st)
}

func (r *Runner) ResizeBatch(target int) error {
	st, err := r.stateStore.Load()
	if err != nil {
		return err
	}
	if st.ActiveBatch == nil {
		return nil
	}
	if target < st.ActiveBatch.CompletedIterations {
		target = st.ActiveBatch.CompletedIterations
	}
	st.ActiveBatch.TargetIterations = target
	return r.stateStore.Save(st)
}

func (r *Runner) Run(ctx context.Context) ([]SessionResult, error) {
	if err := os.MkdirAll(r.cfg.DataDir, 0o755); err != nil {
		return nil, err
	}
	st, err := r.StartOrResumeBatch(r.cfg.Iterations)
	if err != nil {
		return nil, err
	}
	mix, err := ParseAgentMix(r.cfg.AgentSpecs)
	if err != nil {
		return nil, err
	}

	var results []SessionResult
	for st.ActiveBatch != nil && st.ActiveBatch.CompletedIterations < st.ActiveBatch.TargetIterations {
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
		current, err := r.runOne(ctx, &st, mix)
		if err != nil {
			return results, err
		}
		results = append(results, current)
		st, err = r.stateStore.Load()
		if err != nil {
			return results, err
		}
		if st.StopAfterCurrent {
			break
		}
	}
	return results, nil
}

func (r *Runner) runOne(ctx context.Context, st *state.State, mix AgentMix) (SessionResult, error) {
	sessionID := st.NextSessionID
	st.NextSessionID++
	st.ActiveBatch.CompletedIterations++
	iterationIndex := st.ActiveBatch.CompletedIterations
	agent := AgentForSession(sessionID, mix)
	startedAt := time.Now().UTC()

	if err := r.stateStore.Save(*st); err != nil {
		return SessionResult{}, err
	}

	sessionDir, err := progress.EnsureSessionDir(r.cfg.DataDir, sessionID)
	if err != nil {
		return SessionResult{}, err
	}
	transcriptPath := progress.TranscriptPath(r.cfg.DataDir, sessionID)
	logFile, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return SessionResult{}, err
	}
	defer logFile.Close()

	messageIDs, promptBody, err := r.buildPrompt(st.ActiveBatch.BatchID, sessionID)
	if err != nil {
		return SessionResult{}, err
	}

	cmdArgs, suppressStderr, err := BuildAgentCommand(agent, promptBody)
	if err != nil {
		return SessionResult{}, err
	}
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = r.cfg.WorkspaceDir
	cmd.Env = append(os.Environ(),
		contract.EnvDataDir+"="+r.cfg.DataDir,
		contract.EnvRepoProgressPath+"="+r.cfg.RepoProgressPath,
		contract.EnvWorkspaceDir+"="+r.cfg.WorkspaceDir,
		contract.EnvSessionID+"="+strconv.Itoa(sessionID),
		contract.EnvBatchID+"="+strconv.Itoa(st.ActiveBatch.BatchID),
		contract.EnvIterationIndex+"="+strconv.Itoa(iterationIndex),
		contract.EnvAgent+"="+agent,
		contract.EnvSessionDir+"="+sessionDir,
	)

	stdout := io.MultiWriter(logFile, r.cfg.Stdout)
	stderrTarget := io.MultiWriter(logFile, r.cfg.Stderr)
	cmd.Stdout = stdout
	if suppressStderr {
		cmd.Stderr = logFile
	} else {
		cmd.Stderr = stderrTarget
	}

	sessionMeta := progress.SessionMeta{
		Version: contract.SchemaVersion,
		Session: progress.SessionProgress{
			SessionID:      sessionID,
			BatchID:        st.ActiveBatch.BatchID,
			IterationIndex: iterationIndex,
			Agent:          agent,
			Status:         "running",
			StartedAt:      startedAt.Format(time.RFC3339),
			MessageIDs:     messageIDs,
			TranscriptPath: transcriptPath,
		},
	}
	if err := progress.WriteSessionMeta(progress.SessionMetaPath(r.cfg.DataDir, sessionID), sessionMeta); err != nil {
		return SessionResult{}, err
	}

	runErr := cmd.Run()
	endedAt := time.Now().UTC()
	exitCode := 0
	status := "completed"
	if runErr != nil {
		status = "failed"
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	runtimeSeconds := int(endedAt.Sub(startedAt).Seconds())
	if err := progress.UpdateSessionMeta(r.cfg.DataDir, sessionID, func(meta *progress.SessionMeta) error {
		meta.Session.Status = status
		meta.Session.EndedAt = endedAt.Format(time.RFC3339)
		meta.Session.RuntimeSeconds = runtimeSeconds
		return nil
	}); err != nil {
		return SessionResult{}, err
	}

	if st.StopAfterCurrent || st.ActiveBatch.CompletedIterations >= st.ActiveBatch.TargetIterations {
		st.ActiveBatch.EndedAt = endedAt.Format(time.RFC3339)
		st.ActiveBatch = nil
		st.StopAfterCurrent = false
	}
	if err := r.stateStore.Save(*st); err != nil {
		return SessionResult{}, err
	}
	if _, err := progress.RebuildRepoProgress(r.cfg.DataDir, r.cfg.RepoProgressPath, activeBatchMap(st.ActiveBatch)); err != nil {
		return SessionResult{}, err
	}

	return SessionResult{
		SessionID:      sessionID,
		BatchID:        sessionMeta.Session.BatchID,
		IterationIndex: iterationIndex,
		Agent:          agent,
		ExitCode:       exitCode,
	}, runErr
}

func (r *Runner) buildPrompt(batchID, sessionID int) ([]int, string, error) {
	basePrompt := "You are running inside sand-orch. Complete one scoped task thoroughly and use `sand-orch progress record` to update session progress before you exit."
	if data, err := os.ReadFile(filepath.Join(r.cfg.WorkspaceDir, "scripts", "ralph", "prompt.txt")); err == nil {
		basePrompt = strings.TrimSpace(string(data))
	}

	events, err := r.messageStore.Load()
	if err != nil {
		return nil, "", err
	}
	folded := messages.Fold(events)
	ordered := messages.OrderedMessages(folded)

	var batchBodies []string
	var sessionBody string
	var consumed []int
	st, err := r.stateStore.Load()
	if err != nil {
		return nil, "", err
	}

	for _, msg := range ordered {
		switch msg.Scope {
		case messages.ScopeBatch:
			if msg.ApplyBatchID != nil && *msg.ApplyBatchID == batchID && !msg.Canceled {
				batchBodies = append(batchBodies, msg.Body)
				continue
			}
			if !msg.Pending() {
				continue
			}
			target := 0
			if msg.TargetBatchID != nil {
				target = *msg.TargetBatchID
			}
			if target == 0 || target == batchID {
				batchBodies = append(batchBodies, msg.Body)
				applyBatchID := batchID
				if err := r.messageStore.Append(messages.Event{
					EventID:      st.NextEventID,
					MessageID:    msg.MessageID,
					Scope:        messages.ScopeBatch,
					EventType:    messages.EventMessageConsumed,
					ConsumedAt:   messages.Timestamp(),
					ApplyBatchID: &applyBatchID,
				}); err != nil {
					return nil, "", err
				}
				st.NextEventID++
				consumed = append(consumed, msg.MessageID)
			}
		case messages.ScopeSession:
			if !msg.Pending() {
				continue
			}
			if sessionBody == "" {
				sessionBody = msg.Body
				targetSessionID := sessionID
				if err := r.messageStore.Append(messages.Event{
					EventID:         st.NextEventID,
					MessageID:       msg.MessageID,
					Scope:           messages.ScopeSession,
					EventType:       messages.EventMessageConsumed,
					ConsumedAt:      messages.Timestamp(),
					TargetSessionID: &targetSessionID,
				}); err != nil {
					return nil, "", err
				}
				st.NextEventID++
				consumed = append(consumed, msg.MessageID)
			}
		}
	}
	if err := r.stateStore.Save(st); err != nil {
		return nil, "", err
	}

	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(basePrompt))
	builder.WriteString("\n\n## Sand-Orch Context\n")
	if len(batchBodies) > 0 {
		builder.WriteString("### Batch Messages\n")
		for _, body := range batchBodies {
			builder.WriteString("- ")
			builder.WriteString(body)
			builder.WriteString("\n")
		}
	}
	if sessionBody != "" {
		builder.WriteString("### Session Message\n")
		builder.WriteString(sessionBody)
		builder.WriteString("\n")
	}
	return consumed, builder.String(), nil
}

func activeBatchMap(batch *state.BatchState) map[string]any {
	if batch == nil {
		return nil
	}
	return map[string]any{
		"batch_id":             batch.BatchID,
		"target_iterations":    batch.TargetIterations,
		"completed_iterations": batch.CompletedIterations,
		"agent_mix":            batch.AgentMix,
		"started_at":           batch.StartedAt,
		"ended_at":             batch.EndedAt,
	}
}
