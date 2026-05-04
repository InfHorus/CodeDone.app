package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	configSnapshotFile   = "config-snapshot.json"
	repoSnapshotFile     = "repo-snapshot.json"
	sessionFileName      = "session.json"
	rosterFileName       = "roster.json"
	questionsFileName    = "questions.md"
	conversationFileName = "conversation.jsonl"
	planFileName         = "plan.md"
	directiveFileName    = "directive.md"
	backlogFileName      = "backlog.json"
	reviewLogFileName    = "review-log.jsonl"
	finalReportFileName  = "final-report.md"
)

type Service struct {
	rootDir string
	now     func() time.Time

	// Pause/resume gate. The workflow consults `gate(ctx)` at every safe
	// checkpoint (between phases / between tickets). When paused, the gate
	// blocks until Resume is called or ctx is cancelled.
	pauseMu sync.Mutex
	paused  bool
	resume  chan struct{}
}

// Pause asks the workflow to halt at the next safe checkpoint. Already-running
// provider calls finish (we don't kill in-flight HTTP requests), but no new
// phase or ticket starts until Resume is called.
func (s *Service) Pause() {
	s.pauseMu.Lock()
	defer s.pauseMu.Unlock()
	if s.paused {
		return
	}
	s.paused = true
	s.resume = make(chan struct{})
}

// Resume releases the pause gate. Safe to call when not paused.
func (s *Service) Resume() {
	s.pauseMu.Lock()
	defer s.pauseMu.Unlock()
	if !s.paused {
		return
	}
	if s.resume != nil {
		close(s.resume)
	}
	s.resume = nil
	s.paused = false
}

// IsPaused reports whether the gate is currently closed.
func (s *Service) IsPaused() bool {
	s.pauseMu.Lock()
	defer s.pauseMu.Unlock()
	return s.paused
}

// gate blocks while the service is paused. Returns ctx.Err() if the context
// is cancelled while waiting.
func (s *Service) gate(ctx context.Context) error {
	s.pauseMu.Lock()
	if !s.paused {
		s.pauseMu.Unlock()
		return nil
	}
	ch := s.resume
	s.pauseMu.Unlock()
	if ch == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	}
}

func NewService(rootDir string) (*Service, error) {
	if rootDir == "" {
		cfgDir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("resolve user config dir: %w", err)
		}
		rootDir = filepath.Join(cfgDir, "CodeDone", "sessions")
	}

	return &Service{
		rootDir: rootDir,
		now:     func() time.Time { return time.Now().UTC() },
	}, nil
}

func (s *Service) RootDir() string {
	return s.rootDir
}

func (s *Service) Bootstrap(req BootstrapRequest) (*BootstrapResult, error) {
	now := s.now()
	mode := req.Mode
	if mode == "" {
		mode = SessionModeBuild
	}
	if mode == SessionModePlan {
		req.EnableFinalizer = false
		req.AutoCommit = false
	}
	sessionID := newSessionID(now)
	workspacePath := filepath.Join(s.rootDir, sessionID)
	ticketsDir := filepath.Join(workspacePath, "tickets")
	reportsDir := filepath.Join(workspacePath, "reports")

	for _, dir := range []string{workspacePath, ticketsDir, reportsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create session workspace %q: %w", dir, err)
		}
	}

	roster := buildRoster(sessionID, req.CMCount, req.ImplementerMax, req.EnableFinalizer)
	activeCMID := firstCMID(roster)

	session := SessionFile{
		SessionID:          sessionID,
		State:              StateQuestionGate,
		Mode:               mode,
		AutonomyMode:       AutonomyQuestionGate,
		CreatedAt:          now,
		UpdatedAt:          now,
		UserRequest:        req.UserRequest,
		UserRepoPath:       req.UserRepoPath,
		WorkspacePath:      workspacePath,
		ConfigSnapshotFile: configSnapshotFile,
		CMCount:            max(req.CMCount, 1),
		ImplementerMax:     max(req.ImplementerMax, 0),
		AutoCommit:         req.AutoCommit,
		ActiveCMID:         activeCMID,
		QuestionGate: QuestionGateState{
			Needed:          false,
			DecisionPending: true,
			Asked:           false,
			MaxQuestions:    5,
			QuestionCount:   0,
			Answered:        false,
			BlockedOnUser:   false,
		},
		Artifacts: ArtifactPaths{
			QuestionsMD:      questionsFileName,
			DirectiveMD:      directiveFileName,
			RosterJSON:       rosterFileName,
			RepoSnapshotJSON: repoSnapshotFile,
			BacklogJSON:      backlogFileName,
			ReviewLogJSONL:   reviewLogFileName,
			FinalReportMD:    finalReportFileName,
		},
		BacklogSummary: BacklogSummary{},
	}

	backlog := BacklogFile{
		SessionID:     sessionID,
		GeneratedBy:   derefString(activeCMID, "cm-01"),
		GeneratedAt:   now,
		Frozen:        false,
		ScopeSummary:  ScopeSummary{},
		TicketOrder:   []string{},
		ReadyQueue:    []string{},
		BlockedQueue:  []string{},
		DeferredQueue: []string{},
		DoneQueue:     []string{},
		TicketIndex:   map[string]string{},
	}

	if err := writeJSON(filepath.Join(workspacePath, configSnapshotFile), req.ConfigSnapshot); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(workspacePath, repoSnapshotFile), req.RepoSnapshot); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(workspacePath, sessionFileName), session); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(workspacePath, rosterFileName), roster); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(workspacePath, backlogFileName), backlog); err != nil {
		return nil, err
	}
	if err := writeText(filepath.Join(workspacePath, questionsFileName), renderQuestionsTemplate(activeCMID)); err != nil {
		return nil, err
	}
	if err := writeText(filepath.Join(workspacePath, conversationFileName), ""); err != nil {
		return nil, err
	}
	if err := writeText(filepath.Join(workspacePath, planFileName), renderPlanTemplate(session)); err != nil {
		return nil, err
	}
	if err := writeText(filepath.Join(workspacePath, directiveFileName), renderDirectiveTemplate(session, derefString(activeCMID, "cm-01"))); err != nil {
		return nil, err
	}
	if err := writeText(filepath.Join(workspacePath, reviewLogFileName), ""); err != nil {
		return nil, err
	}
	if err := writeText(filepath.Join(workspacePath, finalReportFileName), renderFinalReportTemplate(sessionID)); err != nil {
		return nil, err
	}

	return &BootstrapResult{
		Session:       session,
		Roster:        roster,
		Backlog:       backlog,
		WorkspacePath: workspacePath,
	}, nil
}

func buildRoster(sessionID string, cmCount, implementerMax int, enableFinalizer bool) RosterFile {
	roster := RosterFile{
		SessionID:         sessionID,
		CMAgents:          buildCMAgents(cmCount),
		ImplementerAgents: buildImplementerAgents(implementerMax),
		FinalizerAgents:   []AgentSpec{},
	}
	if enableFinalizer {
		roster.FinalizerAgents = []AgentSpec{
			{ID: "finalizer-01", Active: true},
		}
	}
	return roster
}

func buildCMAgents(count int) []AgentSpec {
	count = max(count, 1)
	agents := make([]AgentSpec, 0, count)
	for i := 1; i <= count; i++ {
		role := "cm_reviewer"
		switch {
		case i == 1:
			role = "cm_initial"
		case i == count:
			role = "cm_dispatcher"
		}
		agents = append(agents, AgentSpec{
			ID:     fmt.Sprintf("cm-%02d", i),
			Role:   role,
			Active: true,
		})
	}
	return agents
}

func buildImplementerAgents(maxAgents int) []AgentSpec {
	if maxAgents <= 0 {
		return []AgentSpec{}
	}

	type profileTemplate struct {
		profile     string
		specialties []string
	}

	templates := []profileTemplate{
		{profile: "frontend", specialties: []string{"ui", "interaction", "layout"}},
		{profile: "backend", specialties: []string{"go", "services", "state"}},
		{profile: "state", specialties: []string{"state", "serialization", "artifacts"}},
		{profile: "tests", specialties: []string{"tests", "validation", "regressions"}},
		{profile: "refactor", specialties: []string{"cleanup", "structure", "safety"}},
		{profile: "data", specialties: []string{"schema", "storage", "migration"}},
		{profile: "integrations", specialties: []string{"providers", "apis", "bindings"}},
		{profile: "platform", specialties: []string{"wails", "desktop", "os"}},
		{profile: "git", specialties: []string{"git", "branching", "diffs"}},
		{profile: "infra", specialties: []string{"tooling", "scripts", "build"}},
	}

	counts := map[string]int{}
	agents := make([]AgentSpec, 0, maxAgents)
	for i := 0; i < maxAgents; i++ {
		template := templates[i%len(templates)]
		counts[template.profile]++
		agents = append(agents, AgentSpec{
			ID:          fmt.Sprintf("impl-%02d", i+1),
			Role:        "implementer",
			Profile:     template.profile,
			Active:      true,
			Specialties: template.specialties,
		})
	}
	return agents
}

func firstCMID(roster RosterFile) *string {
	if len(roster.CMAgents) == 0 {
		return nil
	}
	id := roster.CMAgents[0].ID
	return &id
}

func renderQuestionsTemplate(activeCMID *string) string {
	return strings.Join([]string{
		"# Questions",
		"",
		"## Need For Questions",
		fmt.Sprintf("- Needed: pending"),
		fmt.Sprintf("- Asked by: %s", derefString(activeCMID, "cm-01")),
		"- Question count: 0",
		"",
		"## User Questions",
		"Pending CM1 decision.",
		"",
		"## User Answers",
		"Pending CM1 decision.",
		"",
		"## Locked Decisions",
		"Pending question gate.",
		"",
	}, "\n")
}

func renderDirectiveTemplate(session SessionFile, activeCMID string) string {
	return strings.Join([]string{
		"# CM Directive",
		"",
		fmt.Sprintf("- Session ID: %s", session.SessionID),
		fmt.Sprintf("- Current CM Owner: %s", activeCMID),
		"- Previous CM Owner:",
		"- Directive Version: 1",
		fmt.Sprintf("- Last Updated: %s", session.UpdatedAt.Format(time.RFC3339)),
		"",
		"## Session Identity",
		fmt.Sprintf("- Session ID: %s", session.SessionID),
		fmt.Sprintf("- Current CM Owner: %s", activeCMID),
		"- Directive Version: 1",
		"",
		"## User Request",
		fmt.Sprintf("- Original request: %s", session.UserRequest),
		"- Restated goal:",
		"",
		"## User Answers And Locked Decisions",
		"- Pending CM1 question gate.",
		"",
		"## Assumptions",
		"- Pending CM planning.",
		"",
		"## Product Framing",
		"- Pending CM planning.",
		"",
		"## Competitive / Reference Intelligence",
		"- Pending CM research.",
		"",
		"## Scope Classification",
		"### Required For Category Credibility",
		"- Pending CM planning.",
		"",
		"### Best-In-Class Enhancements",
		"- Pending CM planning.",
		"",
		"### Stretch / Later",
		"- Pending CM planning.",
		"",
		"## Architecture Direction",
		"- Pending CM planning.",
		"",
		"## Full Feature Inventory",
		"- Pending CM planning.",
		"",
		"## Risks And Unknowns",
		"- Pending CM planning.",
		"",
		"## Backlog Strategy",
		"- Pending final CM.",
		"",
		"## Dispatch Notes For Final CM",
		"- Pending final CM.",
		"",
	}, "\n")
}

func renderPlanTemplate(session SessionFile) string {
	return strings.Join([]string{
		"# Plan Memory",
		"",
		fmt.Sprintf("- Session ID: %s", session.SessionID),
		fmt.Sprintf("- Last Updated: %s", session.UpdatedAt.Format(time.RFC3339)),
		"",
		"## User Goal",
		fmt.Sprintf("- %s", session.UserRequest),
		"",
		"## Conversation Summary",
		"- No plan conversation yet.",
		"",
		"## Latest Contre-Maitre Answer",
		"- Pending.",
		"",
	}, "\n")
}

func renderFinalReportTemplate(sessionID string) string {
	return strings.Join([]string{
		"# Final Report",
		"",
		"## Session Summary",
		fmt.Sprintf("- Session ID: %s", sessionID),
		"- Finalizer:",
		"- Final State: pending",
		"",
		"## Original Goal",
		"- Pending finalization.",
		"",
		"## What Was Completed",
		"- Pending finalization.",
		"",
		"## Tickets Completed",
		"- Pending finalization.",
		"",
		"## Deferred Or Blocked Items",
		"- Pending finalization.",
		"",
		"## Validation",
		"- Pending finalization.",
		"",
		"## Risks / Residual Gaps",
		"- Pending finalization.",
		"",
		"## Final Signoff",
		"- Approved: pending",
		"- Reason:",
		"",
	}, "\n")
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %q: %w", path, err)
	}
	return writeBytes(path, data)
}

func writeText(path, content string) error {
	return writeBytes(path, []byte(content))
}

func writeBytes(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

func newSessionID(now time.Time) string {
	return fmt.Sprintf("cd-%s-%04x", now.Format("20060102-150405"), now.UnixNano()&0xffff)
}

func derefString(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
