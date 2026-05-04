package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cmrunner "codedone/internal/cm"
	appconfig "codedone/internal/config"
	"codedone/internal/engine"
	"codedone/internal/gitops"
	"codedone/internal/provider/providererror"
	"codedone/internal/runner"
	"codedone/internal/testrunner"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsWindows "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

type Config = appconfig.Config

type AgentRole string

const (
	RoleSystem       AgentRole = "system"
	RoleContreMaitre AgentRole = "contre-maitre"
	RoleImplementer  AgentRole = "implementer"
	RoleFinalizer    AgentRole = "finalizer"
	RoleUser         AgentRole = "user"
)

type Message struct {
	ID          string    `json:"id"`
	Role        AgentRole `json:"role"`
	ActorID     string    `json:"actorId,omitempty"`
	Label       string    `json:"label,omitempty"`
	Phase       string    `json:"phase,omitempty"`
	TicketID    string    `json:"ticketId,omitempty"`
	Status      string    `json:"status,omitempty"`
	Content     string    `json:"content"`
	Time        time.Time `json:"time"`
	Done        bool      `json:"done"`
	TicketIndex int       `json:"ticketIndex,omitempty"` // 1-based position of this ticket in the dispatch order
	TicketTotal int       `json:"ticketTotal,omitempty"` // total dispatchable tickets in the backlog
	TicketTitle string    `json:"ticketTitle,omitempty"` // human title of the ticket
}

type SessionStatus string

const (
	StatusIdle    SessionStatus = "idle"
	StatusRunning SessionStatus = "running"
	StatusWaiting SessionStatus = "waiting"
	StatusPaused  SessionStatus = "paused"
	StatusDone    SessionStatus = "done"
	StatusError   SessionStatus = "error"
)

type Session struct {
	ID        string        `json:"id"`
	Status    SessionStatus `json:"status"`
	Mode      string        `json:"mode"`
	Branch    string        `json:"branch"`
	WorkDir   string        `json:"workDir"`
	Workspace string        `json:"workspace"`
	Messages  []Message     `json:"messages"`
	CreatedAt time.Time     `json:"createdAt"`
}

type PendingQuestion struct {
	ID        string `json:"id"`
	Prompt    string `json:"prompt"`
	Rationale string `json:"rationale"`
}

type PendingQuestionsPayload struct {
	SessionID string            `json:"sessionId"`
	Summary   string            `json:"summary"`
	Questions []PendingQuestion `json:"questions"`
}

type SessionStatusPayload struct {
	Status    string `json:"status"`
	Label     string `json:"label"`
	StartedAt int64  `json:"startedAt,omitempty"` // unix milliseconds; non-zero while a run is active
}

type TicketSummary struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Status             string   `json:"status"`
	AssignedTo         string   `json:"assignedTo,omitempty"`
	AssignedLabel      string   `json:"assignedLabel,omitempty"`
	Summary            string   `json:"summary"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	Constraints        []string `json:"constraints"`
	Priority           string   `json:"priority"`
	ScopeClass         string   `json:"scopeClass"`
	ParentID           string   `json:"parentId,omitempty"`
}

// ActivityPayload is the wire-shape sent over the Wails "activity" event. It
// mirrors engine.Activity so the frontend can drive the live ticker without
// reaching into the engine package directly.
type ActivityPayload struct {
	Kind       string `json:"kind"`
	Actor      string `json:"actor"`
	ActorLabel string `json:"actorLabel"`
	Role       string `json:"role"`
	Phase      string `json:"phase"`
	TicketID   string `json:"ticketId,omitempty"`
	Tool       string `json:"tool,omitempty"`
	Target     string `json:"target,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Status     string `json:"status,omitempty"`
	Time       int64  `json:"time"` // unix milliseconds
}

type sessionRuntime struct {
	workspacePath string
	gitSvc        *gitops.Service
	workDir       string
	cmRunner      engine.CMRunner
	implRunner    engine.ImplementerRunner
	testRunner    engine.TestRunner
	mode          string
	ctx           context.Context
	cancel        context.CancelFunc
	memoryMux     sync.Mutex
	agentMemory   map[string][]engine.MemoryEvent
}

type App struct {
	ctx              context.Context
	config           *Config
	configMux        sync.RWMutex
	session          *Session
	sessionMux       sync.RWMutex
	engine           *engine.Service
	runtime          *sessionRuntime
	sessionStartedAt time.Time
}

func NewApp() *App {
	cfg := appconfig.Default()
	return &App{
		config: &cfg,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	engineSvc, err := engine.NewService("")
	if err != nil {
		log.Printf("Engine init failed: %v", err)
	} else {
		a.engine = engineSvc
	}

	cfg, err := appconfig.Load(mustConfigPath())
	if err != nil {
		log.Printf("Config load: %v (using defaults)", err)
	} else {
		a.configMux.Lock()
		a.config = &cfg
		a.configMux.Unlock()
	}

	log.Println("CodeDone started")
}

func (a *App) shutdown(_ context.Context) {
	a.configMux.RLock()
	cfg := *a.config
	a.configMux.RUnlock()

	if err := appconfig.Save(mustConfigPath(), cfg); err != nil {
		log.Printf("Config save failed: %v", err)
	}
}

func (a *App) GetConfig() Config {
	a.configMux.RLock()
	defer a.configMux.RUnlock()
	return *a.config
}

func (a *App) SaveConfig(cfg Config) error {
	if cfg.Keys == nil {
		cfg.Keys = make(map[string]string)
	}

	// Preserve server-managed fields the settings panel doesn't control.
	// LastWorkDir is written exclusively by rememberWorkDir/PickDirectory;
	// if the frontend sends a stale value it would clobber the real one.
	a.configMux.RLock()
	if a.config != nil {
		cfg.LastWorkDir = a.config.LastWorkDir
	}
	a.configMux.RUnlock()

	a.configMux.Lock()
	a.config = &cfg
	a.configMux.Unlock()

	return appconfig.Save(mustConfigPath(), cfg)
}

func (a *App) GetSession() *Session {
	a.sessionMux.RLock()
	defer a.sessionMux.RUnlock()
	return a.session
}

func (a *App) StartSession(task string, workDir string, mode string) (*Session, error) {
	a.sessionMux.Lock()
	defer a.sessionMux.Unlock()

	if a.session != nil && (a.session.Status == StatusRunning || a.session.Status == StatusPaused || a.session.Status == StatusWaiting) {
		return nil, fmt.Errorf("a session is already running")
	}
	if a.engine == nil {
		return nil, fmt.Errorf("engine is not available")
	}
	mode = normalizeSessionMode(mode)
	workDir = expandTilde(workDir)

	a.configMux.RLock()
	cfg := *a.config
	a.configMux.RUnlock()

	gitSvc := gitops.NewService(cfg.GitPath)
	repoSnapshot, err := gitSvc.InspectRepo(workDir)
	if err != nil {
		return nil, err
	}
	if !repoSnapshot.IsRepo {
		return nil, fmt.Errorf("working directory is not a git repository")
	}
	if cfg.RequireCleanTree && repoSnapshot.IsRepo && repoSnapshot.Dirty {
		return nil, fmt.Errorf("working tree is not clean")
	}
	a.rememberWorkDir(workDir)

	cmRunner, err := buildCMRunner(cfg, gitSvc, workDir)
	if err != nil {
		return nil, err
	}

	var implRunner engine.ImplementerRunner
	var testRunner engine.TestRunner
	if mode == "build" {
		implRunner, err = buildImplementerRunner(cfg, gitSvc, workDir)
		if err != nil {
			return nil, err
		}
		testRunner = testrunner.NewService()
	}

	var bootstrap *engine.BootstrapResult
	reuseSession := a.session != nil && strings.TrimSpace(a.session.Workspace) != "" && samePath(a.session.WorkDir, workDir)
	if reuseSession {
		bootstrap, err = a.engine.LoadWorkspace(a.session.Workspace)
		if err != nil {
			return nil, err
		}
		bootstrap.Session.Mode = engine.SessionMode(mode)
		bootstrap.Session.AutoCommit = cfg.AutoCommit
		bootstrap.Session.CMCount = cfg.CMCount
		bootstrap.Session.ImplementerMax = cfg.MaxAgents
		if mode == "build" {
			bootstrap.Session.UserRequest = task
			bootstrap.Session.State = engine.StateQuestionGate
			bootstrap.Session.AutonomyMode = engine.AutonomyQuestionGate
		}
		if err := a.engine.SaveSession(bootstrap.WorkspacePath, bootstrap.Session); err != nil {
			return nil, err
		}
	} else {
		bootstrap, err = a.engine.Bootstrap(engine.BootstrapRequest{
			UserRequest:     task,
			UserRepoPath:    workDir,
			ConfigSnapshot:  cfg,
			RepoSnapshot:    repoSnapshot,
			Mode:            engine.SessionMode(mode),
			CMCount:         cfg.CMCount,
			ImplementerMax:  cfg.MaxAgents,
			EnableFinalizer: cfg.EnableFinalizer,
			AutoCommit:      cfg.AutoCommit,
		})
		if err != nil {
			return nil, err
		}
	}

	branchPrefix := cfg.BranchPrefix
	if branchPrefix == "" {
		branchPrefix = "codedone/work-"
	}
	branch := repoSnapshot.HeadBranch
	if branch == "" {
		branch = "no branch"
	}
	if mode == "build" && repoSnapshot.IsRepo && cfg.AutoCreateBranch {
		sessionBranch := fmt.Sprintf("%s%d", branchPrefix, time.Now().Unix())
		branch, err = gitSvc.EnsureBranch(workDir, sessionBranch)
		if err != nil {
			return nil, err
		}
		repoSnapshot, err = gitSvc.InspectRepo(workDir)
		if err != nil {
			return nil, err
		}
		repoSnapshot.SessionBranch = branch
		if err := os.WriteFile(
			filepath.Join(bootstrap.WorkspacePath, bootstrap.Session.Artifacts.RepoSnapshotJSON),
			mustJSON(repoSnapshot),
			0o600,
		); err != nil {
			return nil, err
		}
	}

	if mode == "build" {
		if err := a.engine.AppendConversation(bootstrap.WorkspacePath, engine.ConversationEntry{
			Timestamp: time.Now().UTC(),
			Mode:      engine.SessionModeBuild,
			Role:      "user",
			Content:   task,
		}); err != nil {
			return nil, err
		}
	}

	sess := &Session{
		ID:        bootstrap.Session.SessionID,
		Status:    StatusRunning,
		Mode:      mode,
		Branch:    branch,
		WorkDir:   workDir,
		Workspace: bootstrap.WorkspacePath,
		CreatedAt: bootstrap.Session.CreatedAt,
	}
	if reuseSession && a.session != nil {
		sess.Messages = a.session.Messages
	} else {
		sess.Messages = []Message{{
			ID:      newMessageID(),
			Role:    RoleSystem,
			Content: fmt.Sprintf("Session started in %s mode.\nWorking directory: %s\nBranch: %s\nWorkspace: %s", mode, workDir, branch, bootstrap.WorkspacePath),
			Time:    time.Now(),
			Done:    true,
		}}
	}

	runCtx, cancel := context.WithCancel(a.ctx)
	runtimeState := &sessionRuntime{
		workspacePath: bootstrap.WorkspacePath,
		gitSvc:        gitSvc,
		workDir:       workDir,
		cmRunner:      cmRunner,
		implRunner:    implRunner,
		testRunner:    testRunner,
		mode:          mode,
		ctx:           runCtx,
		cancel:        cancel,
		agentMemory:   make(map[string][]engine.MemoryEvent),
	}
	runtimeState.cmRunner = &memoryCMRunner{base: cmRunner, runtime: runtimeState}
	if implRunner != nil {
		runtimeState.implRunner = &memoryImplementerRunner{base: implRunner, runtime: runtimeState}
	}
	a.session = sess
	a.runtime = runtimeState
	a.sessionStartedAt = time.Now()
	if mode == "plan" {
		a.emitSessionStatus(StatusRunning, "Planning")
		go a.runPlanLoop(runCtx, bootstrap.WorkspacePath, task, runtimeState.cmRunner, !reuseSession)
	} else {
		a.emitSessionStatus(StatusRunning, "Running")
		go a.runAgentLoop(runCtx, bootstrap.WorkspacePath, gitSvc, workDir, runtimeState.cmRunner, runtimeState.implRunner, testRunner, !reuseSession)
	}

	return sess, nil
}

// PauseSession asks the engine to halt at the next safe checkpoint. It is a
// soft pause — already-issued provider calls finish, but no new ticket or
// phase will start until ResumeSession is called.
func (a *App) PauseSession() {
	a.sessionMux.Lock()
	if a.session == nil || a.session.Status != StatusRunning || a.engine == nil {
		a.sessionMux.Unlock()
		return
	}
	a.session.Status = StatusPaused
	a.sessionMux.Unlock()
	a.engine.Pause()
	a.emitSessionStatus(StatusPaused, "Paused")
}

// ResumeSession releases the pause gate so the workflow continues from where
// it left off.
func (a *App) ResumeSession() {
	a.sessionMux.Lock()
	if a.session == nil || a.session.Status != StatusPaused || a.engine == nil {
		a.sessionMux.Unlock()
		return
	}
	a.session.Status = StatusRunning
	a.sessionMux.Unlock()
	a.engine.Resume()
	a.emitSessionStatus(StatusRunning, "Running")
}

func (a *App) CancelSession() {
	a.sessionMux.Lock()
	defer a.sessionMux.Unlock()

	if a.session == nil || a.session.Status != StatusRunning {
		return
	}

	cancelRuntime(a.runtime)
	a.session.Status = StatusIdle
	a.runtime = nil
	if a.engine != nil {
		// Release any pause gate so blocked goroutines can observe ctx
		// cancellation and unwind cleanly.
		a.engine.Resume()
	}
	a.emitMessage(Message{
		ID:      newMessageID(),
		Role:    RoleSystem,
		Content: "Session cancelled.",
		Time:    time.Now(),
		Done:    true,
	})
	a.emitSessionStatus(StatusIdle, "Idle")
	a.sessionStartedAt = time.Time{}
}

func (a *App) ResetSession() {
	a.sessionMux.Lock()
	cancelRuntime(a.runtime)
	a.session = nil
	a.runtime = nil
	a.sessionStartedAt = time.Time{}
	a.sessionMux.Unlock()
	if a.engine != nil {
		a.engine.Resume()
	}
	a.emitSessionStatus(StatusIdle, "Idle")
}

func cancelRuntime(runtimeState *sessionRuntime) {
	if runtimeState != nil && runtimeState.cancel != nil {
		runtimeState.cancel()
	}
}

const (
	maxMemoryEventBytes  = 30 * 1024
	maxMemoryEvents      = 100
	maxMemoryInjectBytes = 60 * 1024
)

type memoryCMRunner struct {
	base    engine.CMRunner
	runtime *sessionRuntime
}

type memoryImplementerRunner struct {
	base    engine.ImplementerRunner
	runtime *sessionRuntime
}

func (r *sessionRuntime) remember(agentID, phase, ticketID string, evt engine.MemoryEvent) {
	if r == nil {
		return
	}
	agentID = strings.TrimSpace(coalesceString(evt.AgentID, agentID))
	if agentID == "" {
		return
	}
	evt.AgentID = agentID
	if evt.Time.IsZero() {
		evt.Time = time.Now().UTC()
	}
	if strings.TrimSpace(evt.Phase) == "" {
		evt.Phase = phase
	}
	if strings.TrimSpace(evt.TicketID) == "" {
		evt.TicketID = ticketID
	}
	evt.Content = limitMemoryText(evt.Content, maxMemoryEventBytes)
	if strings.TrimSpace(evt.Content) == "" {
		return
	}
	r.memoryMux.Lock()
	defer r.memoryMux.Unlock()
	r.agentMemory[agentID] = append(r.agentMemory[agentID], evt)
	if len(r.agentMemory[agentID]) > maxMemoryEvents {
		r.agentMemory[agentID] = append([]engine.MemoryEvent{}, r.agentMemory[agentID][len(r.agentMemory[agentID])-maxMemoryEvents:]...)
	}
}

func (r *sessionRuntime) rememberFunc(agentID, phase, ticketID string) engine.MemoryFunc {
	return func(evt engine.MemoryEvent) {
		r.remember(agentID, phase, ticketID, evt)
	}
}

func (r *sessionRuntime) memoryBlock(agentID string) string {
	if r == nil {
		return ""
	}
	r.memoryMux.Lock()
	events := append([]engine.MemoryEvent{}, r.agentMemory[agentID]...)
	r.memoryMux.Unlock()
	if len(events) == 0 {
		return ""
	}
	roots, guidanceEvents, reviews, recent := []string{}, []string{}, []string{}, []string{}
	for _, evt := range events {
		line := formatMemoryEvent(evt)
		switch evt.Kind {
		case "user_prompt", "dispatch", "session_goal":
			roots = append(roots, line)
		case "guidance", "guidance_get", "guidance_list":
			guidanceEvents = append(guidanceEvents, line)
		case "review", "cm_review":
			reviews = append(reviews, line)
		default:
			recent = append(recent, line)
		}
	}
	parts := []string{
		"Agent Memory:",
		"Historical context only. Re-read relevant files before relying on current repo state.",
	}
	appendSection := func(title string, lines []string) {
		if len(lines) == 0 {
			return
		}
		parts = append(parts, "", title+":")
		parts = append(parts, limitStringList(lines, 20)...)
	}
	appendSection("Session Roots", roots)
	appendSection("Guidance", guidanceEvents)
	appendSection("Reviews", reviews)
	appendSection("Recent", tailStrings(recent, 40))
	return limitMemoryText(strings.Join(parts, "\n"), maxMemoryInjectBytes)
}

func formatMemoryEvent(evt engine.MemoryEvent) string {
	prefix := strings.TrimSpace(evt.Kind)
	if strings.TrimSpace(evt.Phase) != "" {
		prefix += "/" + strings.TrimSpace(evt.Phase)
	}
	if strings.TrimSpace(evt.TicketID) != "" {
		prefix += "/" + strings.TrimSpace(evt.TicketID)
	}
	return "- " + prefix + ": " + strings.TrimSpace(evt.Content)
}

func limitMemoryText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return strings.TrimSpace(s[:max-3]) + "..."
}

func tailStrings(items []string, max int) []string {
	if max <= 0 || len(items) <= max {
		return items
	}
	return append([]string{}, items[len(items)-max:]...)
}

func limitStringList(items []string, max int) []string {
	if max <= 0 || len(items) <= max {
		return items
	}
	return append([]string{}, items[:max]...)
}

func coalesceString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (m *memoryCMRunner) RunPlan(ctx context.Context, req engine.CMPlanRequest) (engine.CMPlanResult, error) {
	agentID := req.CM.ID
	m.runtime.remember(agentID, "plan", "", engine.MemoryEvent{Kind: "user_prompt", Content: req.UserMessage})
	req.AgentMemory = m.runtime.memoryBlock(agentID)
	req.Remember = m.runtime.rememberFunc(agentID, "plan", "")
	result, err := m.base.RunPlan(ctx, req)
	if err == nil {
		m.runtime.remember(agentID, "plan", "", engine.MemoryEvent{Kind: "assistant_answer", Content: result.Answer})
	}
	return result, err
}

func (m *memoryCMRunner) RunQuestionGate(ctx context.Context, req engine.CMQuestionGateRequest) (engine.CMQuestionGateResult, error) {
	agentID := req.CM.ID
	m.runtime.remember(agentID, "question_gate", "", engine.MemoryEvent{Kind: "user_prompt", Content: req.Session.UserRequest})
	req.AgentMemory = m.runtime.memoryBlock(agentID)
	req.Remember = m.runtime.rememberFunc(agentID, "question_gate", "")
	result, err := m.base.RunQuestionGate(ctx, req)
	if err == nil {
		m.runtime.remember(agentID, "question_gate", "", engine.MemoryEvent{Kind: "assistant_answer", Content: result.Summary})
	}
	return result, err
}

func (m *memoryCMRunner) RunDirectivePass(ctx context.Context, req engine.CMDirectiveRequest) (engine.CMDirectiveResult, error) {
	agentID := req.CM.ID
	m.runtime.remember(agentID, req.Phase, "", engine.MemoryEvent{Kind: "session_goal", Content: req.Session.UserRequest})
	req.AgentMemory = m.runtime.memoryBlock(agentID)
	req.Remember = m.runtime.rememberFunc(agentID, req.Phase, "")
	result, err := m.base.RunDirectivePass(ctx, req)
	if err == nil {
		m.runtime.remember(agentID, req.Phase, "", engine.MemoryEvent{Kind: "assistant_answer", Content: result.Summary})
	}
	return result, err
}

func (m *memoryCMRunner) RunBacklogPlan(ctx context.Context, req engine.BacklogPlanRequest) (engine.BacklogPlanResult, error) {
	agentID := req.CM.ID
	m.runtime.remember(agentID, "backlog", "", engine.MemoryEvent{Kind: "session_goal", Content: req.Session.UserRequest})
	req.AgentMemory = m.runtime.memoryBlock(agentID)
	req.Remember = m.runtime.rememberFunc(agentID, "backlog", "")
	result, err := m.base.RunBacklogPlan(ctx, req)
	if err == nil {
		m.runtime.remember(agentID, "backlog", "", engine.MemoryEvent{Kind: "assistant_answer", Content: result.Summary})
	}
	return result, err
}

func (m *memoryCMRunner) RunReview(ctx context.Context, req engine.CMReviewRequest) (engine.CMReviewResult, error) {
	agentID := req.CM.ID
	m.runtime.remember(agentID, "review", req.Ticket.ID, engine.MemoryEvent{Kind: "review", Content: fmt.Sprintf("Reviewing %s after %s reported: %s", req.Ticket.ID, req.Report.ImplementerID, req.Report.Summary)})
	req.AgentMemory = m.runtime.memoryBlock(agentID)
	req.Remember = m.runtime.rememberFunc(agentID, "review", req.Ticket.ID)
	result, err := m.base.RunReview(ctx, req)
	if err == nil {
		m.runtime.remember(agentID, "review", req.Ticket.ID, engine.MemoryEvent{Kind: "cm_review", Content: fmt.Sprintf("%s: %s - %s", result.Decision, result.Summary, result.Reason)})
		m.runtime.remember(req.Report.ImplementerID, "review", req.Ticket.ID, engine.MemoryEvent{Kind: "cm_review", Content: fmt.Sprintf("CM decided %s on %s: %s", result.Decision, req.Ticket.ID, result.Reason)})
	}
	return result, err
}

func (m *memoryCMRunner) RunFinalizer(ctx context.Context, req engine.FinalizerRequest) (engine.FinalizerResult, error) {
	agentID := "finalizer-01"
	req.AgentMemory = m.runtime.memoryBlock(agentID)
	req.Remember = m.runtime.rememberFunc(agentID, "finalizer", "")
	result, err := m.base.RunFinalizer(ctx, req)
	if err == nil {
		m.runtime.remember(agentID, "finalizer", "", engine.MemoryEvent{Kind: "assistant_answer", Content: result.Summary})
	}
	return result, err
}

func (m *memoryImplementerRunner) RunImplementer(ctx context.Context, req engine.ImplementerRunRequest) (engine.ImplementerRunResult, error) {
	agentID := req.Implementer.ID
	m.runtime.remember(agentID, "implementing", req.Ticket.ID, engine.MemoryEvent{
		Kind: "dispatch",
		Content: fmt.Sprintf("%s: %s\n%s\nAcceptance: %s",
			req.Ticket.ID,
			req.Ticket.Title,
			req.Ticket.Summary,
			strings.Join(req.Ticket.AcceptanceCriteria, "; "),
		),
	})
	req.AgentMemory = m.runtime.memoryBlock(agentID)
	req.Remember = m.runtime.rememberFunc(agentID, "implementing", req.Ticket.ID)
	result, err := m.base.RunImplementer(ctx, req)
	if err == nil {
		m.runtime.remember(agentID, "implementing", req.Ticket.ID, engine.MemoryEvent{
			Kind:    "assistant_answer",
			Content: fmt.Sprintf("%s\nFiles changed: %s\nRisks: %s", result.Summary, strings.Join(result.FilesChanged, ", "), strings.Join(result.KnownRisks, " | ")),
		})
	}
	return result, err
}

func (a *App) runAgentLoop(ctx context.Context, workspacePath string, gitSvc *gitops.Service, workDir string, cmRunner engine.CMRunner, implRunner engine.ImplementerRunner, testRunner engine.TestRunner, announceWorkspace bool) {
	if announceWorkspace {
		a.emitSessionMessage(workspacePath, Message{
			ID:   newMessageID(),
			Role: RoleSystem,
			Content: fmt.Sprintf(
				"Private session workspace ready.\nDirective: %s\nBacklog: %s",
				filepath.Join(workspacePath, "directive.md"),
				filepath.Join(workspacePath, "backlog.json"),
			),
			Time: time.Now(),
			Done: true,
		})
	}

	commitFn := engine.SessionCommitFunc(func(repoPath, message string) (string, error) {
		return gitSvc.Commit(repoPath, message)
	})
	err := a.engine.RunSession(ctx, workspacePath, cmRunner, implRunner, testRunner, commitFn, func(evt engine.Event) {
		a.emitSessionMessage(workspacePath, Message{
			ID:          messageIDForEvent(evt),
			Role:        mapEngineRole(evt.Role),
			ActorID:     evt.ActorID,
			Label:       displayActorLabel(mapEngineRole(evt.Role), evt.ActorID),
			Phase:       evt.Phase,
			TicketID:    evt.TicketID,
			Status:      string(evt.Status),
			Content:     evt.Content,
			Time:        time.Now(),
			Done:        evt.Status != engine.EventRunning,
			TicketIndex: evt.TicketIndex,
			TicketTotal: evt.TicketTotal,
			TicketTitle: evt.TicketTitle,
		})
		if evt.Status == engine.EventDone {
			a.emitBacklogUpdate(workspacePath)
		}
	}, func(act engine.Activity) {
		a.emitSessionActivity(workspacePath, act)
	})

	if ctx.Err() != nil {
		return
	}

	if errors.Is(err, engine.ErrAwaitingUserInput) {
		a.sessionMux.Lock()
		if a.session != nil {
			a.session.Status = StatusWaiting
		}
		a.sessionMux.Unlock()
		a.emitSessionStatus(StatusWaiting, "Awaiting input")
		if payload, payloadErr := a.loadPendingQuestionsPayload(workspacePath); payloadErr == nil {
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "questions_pending", payload)
			}
		}
		return
	}

	if err != nil {
		a.sessionMux.Lock()
		if a.session != nil {
			a.session.Status = StatusError
		}
		a.sessionMux.Unlock()
		a.emitSessionMessage(workspacePath, Message{
			ID:      newMessageID(),
			Role:    RoleSystem,
			Content: formatUserFacingError("Session failed", err),
			Time:    time.Now(),
			Done:    true,
		})
		a.emitSessionStatus(StatusError, "Error")
		a.sessionStartedAt = time.Time{}
		return
	}

	a.sessionMux.Lock()
	if a.session != nil {
		a.session.Status = StatusDone
	}
	a.runtime = nil
	a.sessionMux.Unlock()
	a.emitSessionStatus(StatusDone, "Done")
	a.sessionStartedAt = time.Time{}
}

func (a *App) runPlanLoop(ctx context.Context, workspacePath, userMessage string, cmRunner engine.CMRunner, announceWorkspace bool) {
	if announceWorkspace {
		a.emitSessionMessage(workspacePath, Message{
			ID:   newMessageID(),
			Role: RoleSystem,
			Content: fmt.Sprintf(
				"Plan workspace ready.\nPlan memory: %s\nConversation: %s",
				filepath.Join(workspacePath, "plan.md"),
				filepath.Join(workspacePath, "conversation.jsonl"),
			),
			Time: time.Now(),
			Done: true,
		})
	}

	err := a.engine.RunPlan(ctx, workspacePath, userMessage, cmRunner, func(evt engine.Event) {
		a.emitSessionMessage(workspacePath, Message{
			ID:      messageIDForEvent(evt),
			Role:    mapEngineRole(evt.Role),
			ActorID: evt.ActorID,
			Label:   displayActorLabel(mapEngineRole(evt.Role), evt.ActorID),
			Phase:   evt.Phase,
			Status:  string(evt.Status),
			Content: evt.Content,
			Time:    time.Now(),
			Done:    evt.Status != engine.EventRunning,
		})
	}, func(act engine.Activity) {
		a.emitSessionActivity(workspacePath, act)
	})

	if ctx.Err() != nil {
		return
	}

	if err != nil {
		a.sessionMux.Lock()
		if a.session != nil {
			a.session.Status = StatusError
		}
		a.sessionMux.Unlock()
		a.emitSessionMessage(workspacePath, Message{
			ID:      newMessageID(),
			Role:    RoleSystem,
			Content: formatUserFacingError("Plan failed", err),
			Time:    time.Now(),
			Done:    true,
		})
		a.emitSessionStatus(StatusError, "Error")
		a.sessionStartedAt = time.Time{}
		return
	}

	a.sessionMux.Lock()
	if a.session != nil {
		a.session.Status = StatusDone
		a.session.Mode = "plan"
	}
	a.sessionMux.Unlock()
	a.emitSessionStatus(StatusDone, "Plan done")
	a.sessionStartedAt = time.Time{}
}

func (a *App) GetPendingQuestions() (*PendingQuestionsPayload, error) {
	a.sessionMux.RLock()
	workspacePath := ""
	if a.runtime != nil {
		workspacePath = a.runtime.workspacePath
	} else if a.session != nil {
		workspacePath = a.session.Workspace
	}
	a.sessionMux.RUnlock()
	if strings.TrimSpace(workspacePath) == "" {
		return nil, fmt.Errorf("no active session workspace")
	}
	return a.loadPendingQuestionsPayload(workspacePath)
}

func (a *App) SubmitQuestionAnswers(answers []string) error {
	a.sessionMux.Lock()
	if a.session == nil || a.session.Status != StatusWaiting {
		a.sessionMux.Unlock()
		return fmt.Errorf("session is not waiting for question answers")
	}
	runtimeState := a.runtime
	if runtimeState == nil {
		a.sessionMux.Unlock()
		return fmt.Errorf("session runtime is unavailable")
	}
	a.session.Status = StatusRunning
	a.sessionMux.Unlock()

	if _, err := a.engine.SubmitQuestionAnswers(runtimeState.workspacePath, answers); err != nil {
		a.sessionMux.Lock()
		if a.session != nil {
			a.session.Status = StatusWaiting
		}
		a.sessionMux.Unlock()
		return err
	}
	a.emitSessionStatus(StatusRunning, "Running")

	a.emitMessage(Message{
		ID:      newMessageID(),
		Role:    RoleUser,
		Content: strings.Join(answers, "\n"),
		Time:    time.Now(),
		Done:    true,
	})

	go a.runAgentLoop(runtimeState.ctx, runtimeState.workspacePath, runtimeState.gitSvc, runtimeState.workDir, runtimeState.cmRunner, runtimeState.implRunner, runtimeState.testRunner, false)
	return nil
}

func (a *App) loadPendingQuestionsPayload(workspacePath string) (*PendingQuestionsPayload, error) {
	bootstrap, err := a.engine.LoadWorkspace(workspacePath)
	if err != nil {
		return nil, err
	}
	batch := bootstrap.Session.QuestionBatch
	payload := &PendingQuestionsPayload{
		SessionID: bootstrap.Session.SessionID,
		Summary:   batch.Summary,
		Questions: make([]PendingQuestion, 0, len(batch.Questions)),
	}
	for _, q := range batch.Questions {
		payload.Questions = append(payload.Questions, PendingQuestion{
			ID:        q.ID,
			Prompt:    q.Prompt,
			Rationale: q.Rationale,
		})
	}
	return payload, nil
}

func (a *App) emitMessage(msg Message) {
	a.sessionMux.Lock()
	if a.session != nil {
		replaced := false
		for i := range a.session.Messages {
			if a.session.Messages[i].ID == msg.ID {
				a.session.Messages[i] = msg
				replaced = true
				break
			}
		}
		if !replaced {
			a.session.Messages = append(a.session.Messages, msg)
		}
	}
	a.sessionMux.Unlock()

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "message", msg)
	}
}

func (a *App) emitSessionMessage(workspacePath string, msg Message) {
	if !a.isActiveWorkspace(workspacePath) {
		return
	}
	a.emitMessage(msg)
}

func (a *App) emitSessionActivity(workspacePath string, act engine.Activity) {
	if !a.isActiveWorkspace(workspacePath) {
		return
	}
	a.emitActivity(act)
}

func (a *App) isActiveWorkspace(workspacePath string) bool {
	a.sessionMux.RLock()
	defer a.sessionMux.RUnlock()
	return a.session != nil && samePath(a.session.Workspace, workspacePath)
}

func (a *App) PickDirectory() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Select working directory",
		DefaultDirectory:     a.GetWorkDir(),
		CanCreateDirectories: true,
	})
	if err != nil {
		return "", err
	}
	a.rememberWorkDir(dir)
	return dir, nil
}

func (a *App) GetCurrentBranch(workDir string) string {
	if workDir == "" {
		workDir = a.GetWorkDir()
	}
	a.configMux.RLock()
	gitPath := ""
	if a.config != nil {
		gitPath = a.config.GitPath
	}
	a.configMux.RUnlock()

	svc := gitops.NewService(gitPath)
	snap, err := svc.InspectRepo(workDir)
	if err != nil || !snap.IsRepo {
		return ""
	}
	if snap.HeadBranch != "" {
		return snap.HeadBranch
	}
	if snap.HeadCommit != "" && len(snap.HeadCommit) >= 7 {
		return snap.HeadCommit[:7]
	}
	return ""
}

func (a *App) GetTickets() ([]TicketSummary, error) {
	a.sessionMux.RLock()
	workspace := ""
	if a.session != nil {
		workspace = a.session.Workspace
	}
	a.sessionMux.RUnlock()
	if workspace == "" {
		return []TicketSummary{}, nil
	}
	return a.getTicketsForWorkspace(workspace)
}

func (a *App) getTicketsForWorkspace(workspacePath string) ([]TicketSummary, error) {
	backlogPath := filepath.Join(workspacePath, "backlog.json")
	data, err := os.ReadFile(backlogPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []TicketSummary{}, nil
		}
		return nil, err
	}
	var backlog engine.BacklogFile
	if err := json.Unmarshal(data, &backlog); err != nil {
		return nil, err
	}
	summaries := make([]TicketSummary, 0, len(backlog.TicketOrder))
	for _, id := range backlog.TicketOrder {
		ticketData, err := os.ReadFile(filepath.Join(workspacePath, "tickets", id+".json"))
		if err != nil {
			continue
		}
		var ticket engine.TicketFile
		if err := json.Unmarshal(ticketData, &ticket); err != nil {
			continue
		}
		s := TicketSummary{
			ID:                 ticket.ID,
			Title:              ticket.Title,
			Status:             string(ticket.Status),
			AssignedTo:         ticket.AssignedTo,
			Summary:            ticket.Summary,
			AcceptanceCriteria: ticket.AcceptanceCriteria,
			Constraints:        ticket.Constraints,
			Priority:           ticket.Priority,
			ScopeClass:         ticket.ScopeClass,
		}
		if ticket.AssignedTo != "" {
			s.AssignedLabel = actorLabel(RoleImplementer, ticket.AssignedTo)
		}
		if ticket.ParentTicketID != nil {
			s.ParentID = *ticket.ParentTicketID
		}
		summaries = append(summaries, s)
	}
	return summaries, nil
}

func (a *App) emitBacklogUpdate(workspacePath string) {
	tickets, err := a.getTicketsForWorkspace(workspacePath)
	if err != nil || a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "backlog_update", tickets)
}

func (a *App) GetWorkDir() string {
	a.configMux.RLock()
	if a.config != nil && strings.TrimSpace(a.config.LastWorkDir) != "" {
		dir := strings.TrimSpace(a.config.LastWorkDir)
		a.configMux.RUnlock()
		return dir
	}
	a.configMux.RUnlock()

	home, err := os.UserHomeDir()
	if err != nil {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		return wd
	}
	return home
}

func expandTilde(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" || path == "~/" || path == `~\` {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func (a *App) rememberWorkDir(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	a.configMux.Lock()
	if a.config == nil {
		cfg := appconfig.Default()
		a.config = &cfg
	}
	if a.config.LastWorkDir == dir {
		a.configMux.Unlock()
		return
	}
	a.config.LastWorkDir = dir
	cfg := *a.config
	a.configMux.Unlock()
	if err := appconfig.Save(mustConfigPath(), cfg); err != nil {
		log.Printf("Remember workdir failed: %v", err)
	}
}

func mustConfigPath() string {
	dir, err := os.Getwd()
	if err != nil {
		return "codedone.config.json"
	}
	return filepath.Join(dir, "codedone.config.json")
}

func normalizeSessionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "plan":
		return "plan"
	case "build":
		return "build"
	default:
		return "build"
	}
}

func samePath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func newMessageID() string {
	return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}

func mapEngineRole(role string) AgentRole {
	switch role {
	case "contre-maitre":
		return RoleContreMaitre
	case "implementer":
		return RoleImplementer
	case "finalizer":
		return RoleFinalizer
	default:
		return RoleSystem
	}
}

func messageIDForEvent(evt engine.Event) string {
	if strings.TrimSpace(evt.ID) != "" {
		return evt.ID
	}
	return newMessageID()
}

func actorLabel(role AgentRole, actorID string) string {
	actorID = strings.TrimSpace(actorID)
	switch role {
	case RoleContreMaitre:
		if strings.HasPrefix(actorID, "cm-") {
			return "Contre-Maître " + zeroPad2(strings.TrimPrefix(actorID, "cm-"))
		}
		return "Contre-Maître"
	case RoleImplementer:
		if strings.HasPrefix(actorID, "impl-") {
			parts := strings.Split(actorID, "-")
			if len(parts) >= 3 {
				profile := strings.Title(parts[1])
				return fmt.Sprintf("Implementer %s-%s", profile, zeroPad2(parts[len(parts)-1]))
			}
		}
		return "Implementer"
	case RoleFinalizer:
		if strings.HasPrefix(actorID, "finalizer-") {
			return "Finalizer " + zeroPad2(strings.TrimPrefix(actorID, "finalizer-"))
		}
		return "Finalizer"
	case RoleUser:
		return "You"
	default:
		return "System"
	}
}

func displayActorLabel(role AgentRole, actorID string) string {
	actorID = strings.TrimSpace(actorID)
	switch role {
	case RoleContreMaitre:
		if strings.HasPrefix(actorID, "cm-") {
			return "Contre-Maitre " + zeroPad2(strings.TrimPrefix(actorID, "cm-"))
		}
		return "Contre-Maitre"
	case RoleImplementer:
		if strings.HasPrefix(actorID, "impl-") {
			return "Implementer " + zeroPad2(strings.TrimPrefix(actorID, "impl-"))
		}
		return "Implementer"
	case RoleFinalizer:
		if strings.HasPrefix(actorID, "finalizer-") {
			return "Finalizer " + zeroPad2(strings.TrimPrefix(actorID, "finalizer-"))
		}
		return "Finalizer"
	case RoleUser:
		return "You"
	default:
		return "System"
	}
}

func zeroPad2(s string) string {
	n := strings.TrimLeft(s, "0")
	if n == "" {
		n = "0"
	}
	if len(n) == 1 {
		return "0" + n
	}
	return n
}

func formatUserFacingError(title string, err error) string {
	if err == nil {
		return title
	}
	var providerErr *providererror.Error
	if errors.As(err, &providerErr) {
		lines := []string{
			fmt.Sprintf("**%s**", title),
			"",
			providerErr.UserMessage(),
		}
		if fix := providerErrorFix(providerErr); fix != "" {
			lines = append(lines, "", "**What to do**", fix)
		}
		if detail := strings.TrimSpace(providerErr.Detail); detail != "" {
			lines = append(lines, "", "**Provider detail**", detail)
		}
		return strings.Join(lines, "\n")
	}
	return fmt.Sprintf("**%s**\n\n%s", title, strings.TrimSpace(err.Error()))
}

func providerErrorFix(err *providererror.Error) string {
	switch err.Kind {
	case providererror.KindAuthentication:
		return "Open Settings, verify the provider API key, then start the session again."
	case providererror.KindRateLimit:
		return "Wait a moment, reduce implementer slots, or use a provider account with higher limits."
	case providererror.KindQuota:
		return "Add credits to the provider account or switch to another provider/key."
	case providererror.KindModel:
		return "Check the CM and implementer model names in Settings. The current key may not have access to that model."
	case providererror.KindBadRequest:
		return "Check the selected model and max output token setting."
	case providererror.KindNetwork:
		return "Check your connection, proxy, VPN, or firewall, then retry."
	case providererror.KindUnavailable:
		return "Retry later or temporarily switch providers."
	default:
		return ""
	}
}

func (a *App) emitSessionStatus(status SessionStatus, label string) {
	if a.ctx == nil {
		return
	}
	payload := SessionStatusPayload{
		Status: string(status),
		Label:  label,
	}
	if !a.sessionStartedAt.IsZero() {
		payload.StartedAt = a.sessionStartedAt.UnixMilli()
	}
	runtime.EventsEmit(a.ctx, "session_status", payload)
}

func (a *App) emitActivity(act engine.Activity) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "activity", ActivityPayload{
		Kind:       act.Kind,
		Actor:      act.Actor,
		ActorLabel: displayActorLabel(mapEngineRole(act.Role), act.Actor),
		Role:       string(mapEngineRole(act.Role)),
		Phase:      act.Phase,
		TicketID:   act.TicketID,
		Tool:       act.Tool,
		Target:     act.Target,
		Detail:     act.Detail,
		Status:     act.Status,
		Time:       time.Now().UnixMilli(),
	})
}

func buildImplementerRunner(cfg Config, gitSvc *gitops.Service, workDir string) (engine.ImplementerRunner, error) {
	apiKey := resolveAPIKey(cfg)
	switch cfg.Provider {
	case appconfig.ProviderDeepseek:
		if apiKey == "" {
			return nil, providererror.New("DeepSeek", providererror.KindAuthentication, "DeepSeek API key is missing. Add it in Settings before starting a session.")
		}
		model := strings.TrimSpace(cfg.ImplementerModel)
		if model == "" {
			model = strings.TrimSpace(cfg.Model)
		}
		if model == "" {
			model = "deepseek-v4-flash"
		}
		return runner.NewDeepSeekRunner(apiKey, gitSvc, workDir, model, cfg.MaxTokens, generationTemperature(cfg), agentTimeoutDuration(cfg)), nil
	case appconfig.ProviderOpenRouter:
		if apiKey == "" {
			return nil, providererror.New("OpenRouter", providererror.KindAuthentication, "OpenRouter API key is missing. Add it in Settings before starting a session.")
		}
		model := strings.TrimSpace(cfg.ImplementerModel)
		if model == "" {
			model = strings.TrimSpace(cfg.Model)
		}
		if model == "" {
			model = "deepseek/deepseek-chat"
		}
		return runner.NewOpenRouterRunner(apiKey, gitSvc, workDir, model, cfg.MaxTokens, generationTemperature(cfg), agentTimeoutDuration(cfg)), nil
	case appconfig.ProviderOpenAI:
		if apiKey == "" {
			return nil, providererror.New("OpenAI", providererror.KindAuthentication, "OpenAI API key is missing. Add it in Settings before starting a session.")
		}
		model := strings.TrimSpace(cfg.ImplementerModel)
		if model == "" {
			model = strings.TrimSpace(cfg.Model)
		}
		if model == "" {
			model = "gpt-5.4-mini"
		}
		return runner.NewOpenAIRunner(apiKey, gitSvc, workDir, model, cfg.MaxTokens, generationTemperature(cfg), agentTimeoutDuration(cfg)), nil
	case appconfig.ProviderAnthropic:
		if apiKey == "" {
			return nil, providererror.New("Anthropic", providererror.KindAuthentication, "Anthropic API key is missing. Add it in Settings before starting a session.")
		}
		model := strings.TrimSpace(cfg.ImplementerModel)
		if model == "" {
			model = strings.TrimSpace(cfg.Model)
		}
		if model == "" {
			model = "claude-sonnet-4-6"
		}
		return runner.NewAnthropicRunner(apiKey, gitSvc, workDir, model, cfg.MaxTokens, generationTemperature(cfg), agentTimeoutDuration(cfg)), nil
	default:
		return nil, fmt.Errorf("provider %q is not implemented yet", cfg.Provider)
	}
}

func buildCMRunner(cfg Config, gitSvc *gitops.Service, workDir string) (engine.CMRunner, error) {
	apiKey := resolveAPIKey(cfg)
	switch cfg.Provider {
	case appconfig.ProviderDeepseek:
		if apiKey == "" {
			return nil, providererror.New("DeepSeek", providererror.KindAuthentication, "DeepSeek API key is missing. Add it in Settings before starting a session.")
		}
		model := strings.TrimSpace(cfg.CMModel)
		if model == "" {
			model = strings.TrimSpace(cfg.Model)
		}
		if model == "" {
			model = "deepseek-v4-pro"
		}
		return cmrunner.NewDeepSeekRunner(apiKey, gitSvc, workDir, model, cfg.MaxTokens, generationTemperature(cfg), agentTimeoutDuration(cfg)), nil
	case appconfig.ProviderOpenRouter:
		if apiKey == "" {
			return nil, providererror.New("OpenRouter", providererror.KindAuthentication, "OpenRouter API key is missing. Add it in Settings before starting a session.")
		}
		model := strings.TrimSpace(cfg.CMModel)
		if model == "" {
			model = strings.TrimSpace(cfg.Model)
		}
		if model == "" {
			model = "anthropic/claude-sonnet-4.6"
		}
		return cmrunner.NewOpenRouterRunner(apiKey, gitSvc, workDir, model, cfg.MaxTokens, generationTemperature(cfg), agentTimeoutDuration(cfg)), nil
	case appconfig.ProviderOpenAI:
		if apiKey == "" {
			return nil, providererror.New("OpenAI", providererror.KindAuthentication, "OpenAI API key is missing. Add it in Settings before starting a session.")
		}
		model := strings.TrimSpace(cfg.CMModel)
		if model == "" {
			model = strings.TrimSpace(cfg.Model)
		}
		if model == "" {
			model = "gpt-5.4"
		}
		return cmrunner.NewOpenAIRunner(apiKey, gitSvc, workDir, model, cfg.MaxTokens, generationTemperature(cfg), agentTimeoutDuration(cfg)), nil
	case appconfig.ProviderAnthropic:
		if apiKey == "" {
			return nil, providererror.New("Anthropic", providererror.KindAuthentication, "Anthropic API key is missing. Add it in Settings before starting a session.")
		}
		model := strings.TrimSpace(cfg.CMModel)
		if model == "" {
			model = strings.TrimSpace(cfg.Model)
		}
		if model == "" {
			model = "claude-opus-4-7"
		}
		return cmrunner.NewAnthropicRunner(apiKey, gitSvc, workDir, model, cfg.MaxTokens, generationTemperature(cfg), agentTimeoutDuration(cfg)), nil
	default:
		return nil, fmt.Errorf("provider %q is not implemented yet", cfg.Provider)
	}
}

func agentTimeoutDuration(cfg Config) time.Duration {
	if cfg.AgentTimeout <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(cfg.AgentTimeout) * time.Second
}

func generationTemperature(cfg Config) float64 {
	if !cfg.EnableTemperature {
		return 0
	}
	return cfg.Temperature
}

func resolveAPIKey(cfg Config) string {
	if strings.TrimSpace(cfg.APIKey) != "" {
		return strings.TrimSpace(cfg.APIKey)
	}
	if strings.TrimSpace(cfg.Keys[string(cfg.Provider)]) != "" {
		return strings.TrimSpace(cfg.Keys[string(cfg.Provider)])
	}
	if env := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")); env != "" && cfg.Provider == appconfig.ProviderDeepseek {
		return env
	}
	if env := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")); env != "" && cfg.Provider == appconfig.ProviderOpenRouter {
		return env
	}
	if env := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); env != "" && cfg.Provider == appconfig.ProviderOpenAI {
		return env
	}
	if env := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); env != "" && cfg.Provider == appconfig.ProviderAnthropic {
		return env
	}
	return ""
}

func cfgForRunner(a *App) Config {
	a.configMux.RLock()
	defer a.configMux.RUnlock()
	return *a.config
}

func mustJSON(v any) []byte {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return data
}

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "CodeDone",
		Width:            980,
		Height:           720,
		MinWidth:         720,
		MinHeight:        500,
		StartHidden:      false,
		AlwaysOnTop:      false,
		DisableResize:    false,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 10, G: 10, B: 10, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Windows: &wailsWindows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			BackdropType:         wailsWindows.Acrylic,
			DisableWindowIcon:    false,
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}
