package engine

import (
	"context"
	"time"
)

type SessionState string

const (
	StateCreated         SessionState = "created"
	StateQuestionGate    SessionState = "question_gate"
	StatePlanning        SessionState = "planning"
	StateDirectiveReview SessionState = "directive_review"
	StateBacklogFrozen   SessionState = "backlog_frozen"
	StateDispatchReady   SessionState = "dispatch_ready"
	StateImplementing    SessionState = "implementing"
	StateCMReview        SessionState = "cm_review"
	StateFinalizing      SessionState = "finalizing"
	StateDone            SessionState = "done"
	StateBlocked         SessionState = "blocked"
	StateError           SessionState = "error"
	StateCancelled       SessionState = "cancelled"
)

type SessionMode string

const (
	SessionModePlan  SessionMode = "plan"
	SessionModeBuild SessionMode = "build"
)

type AutonomyMode string

const (
	AutonomyQuestionGate  AutonomyMode = "question_gate"
	AutonomyAutonomous    AutonomyMode = "autonomous"
	AutonomyWaitingOnUser AutonomyMode = "waiting_on_user"
)

type QuestionGateState struct {
	Needed          bool `json:"needed"`
	DecisionPending bool `json:"decision_pending"`
	Asked           bool `json:"asked"`
	MaxQuestions    int  `json:"max_questions"`
	QuestionCount   int  `json:"question_count"`
	Answered        bool `json:"answered"`
	BlockedOnUser   bool `json:"blocked_on_user"`
}

type QuestionItem struct {
	ID        string `json:"id"`
	Prompt    string `json:"prompt"`
	Rationale string `json:"rationale,omitempty"`
}

type QuestionAnswer struct {
	QuestionID string `json:"question_id"`
	Answer     string `json:"answer"`
}

type QuestionBatch struct {
	AskedBy         string           `json:"asked_by"`
	Summary         string           `json:"summary"`
	Questions       []QuestionItem   `json:"questions"`
	Answers         []QuestionAnswer `json:"answers"`
	LockedDecisions []string         `json:"locked_decisions"`
	Assumptions     []string         `json:"assumptions"`
}

type ArtifactPaths struct {
	QuestionsMD      string `json:"questions_md"`
	DirectiveMD      string `json:"directive_md"`
	RosterJSON       string `json:"roster_json"`
	RepoSnapshotJSON string `json:"repo_snapshot_json"`
	BacklogJSON      string `json:"backlog_json"`
	ReviewLogJSONL   string `json:"review_log_jsonl"`
	FinalReportMD    string `json:"final_report_md"`
}

type BacklogSummary struct {
	Total      int `json:"total"`
	Todo       int `json:"todo"`
	InProgress int `json:"in_progress"`
	Done       int `json:"done"`
	Blocked    int `json:"blocked"`
	Deferred   int `json:"deferred"`
}

type SessionFile struct {
	SessionID          string            `json:"session_id"`
	State              SessionState      `json:"state"`
	Mode               SessionMode       `json:"mode,omitempty"`
	AutonomyMode       AutonomyMode      `json:"autonomy_mode"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	UserRequest        string            `json:"user_request"`
	UserRepoPath       string            `json:"user_repo_path"`
	WorkspacePath      string            `json:"workspace_path"`
	ConfigSnapshotFile string            `json:"config_snapshot_file"`
	CMCount            int               `json:"cm_count"`
	ImplementerMax     int               `json:"implementer_max"`
	AutoCommit         bool              `json:"auto_commit"`
	SessionBranch      string            `json:"session_branch,omitempty"`
	CommitSHA          string            `json:"commit_sha,omitempty"`
	ActiveCMID         *string           `json:"active_cm_id"`
	ActiveAgentID      *string           `json:"active_agent_id"`
	ActiveTicketID     *string           `json:"active_ticket_id"`
	QuestionGate       QuestionGateState `json:"question_gate"`
	QuestionBatch      QuestionBatch     `json:"question_batch"`
	Artifacts          ArtifactPaths     `json:"artifacts"`
	BacklogSummary     BacklogSummary    `json:"backlog_summary"`
	LastError          *string           `json:"last_error"`
}

type AgentSpec struct {
	ID          string   `json:"id"`
	Role        string   `json:"role,omitempty"`
	Profile     string   `json:"profile,omitempty"`
	Active      bool     `json:"active"`
	Specialties []string `json:"specialties,omitempty"`
}

type RosterFile struct {
	SessionID         string      `json:"session_id"`
	CMAgents          []AgentSpec `json:"cm_agents"`
	ImplementerAgents []AgentSpec `json:"implementer_agents"`
	FinalizerAgents   []AgentSpec `json:"finalizer_agents"`
}

type ScopeSummary struct {
	Required    int `json:"required"`
	BestInClass int `json:"best_in_class"`
	Stretch     int `json:"stretch"`
}

type TicketState string

const (
	TicketTodo       TicketState = "todo"
	TicketInProgress TicketState = "in_progress"
	TicketDone       TicketState = "done"
	TicketBlocked    TicketState = "blocked"
	TicketDeferred   TicketState = "deferred"
	TicketCancelled  TicketState = "cancelled"
)

type ReviewDecision string

const (
	DecisionAccept ReviewDecision = "accept"
	DecisionRevise ReviewDecision = "revise"
	DecisionSplit  ReviewDecision = "split"
	DecisionDefer  ReviewDecision = "defer"
	DecisionBlock  ReviewDecision = "block"
)

type ReportStatus string

const (
	ReportDone    ReportStatus = "done"
	ReportPartial ReportStatus = "partial"
	ReportBlocked ReportStatus = "blocked"
	ReportFailed  ReportStatus = "failed"
)

type TicketFile struct {
	ID                  string      `json:"id"`
	Title               string      `json:"title"`
	Type                string      `json:"type"`
	Area                string      `json:"area"`
	ScopeClass          string      `json:"scope_class"`
	Priority            string      `json:"priority"`
	Status              TicketState `json:"status"`
	CreatedBy           string      `json:"created_by"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
	ParentTicketID      *string     `json:"parent_ticket_id"`
	DependsOn           []string    `json:"depends_on"`
	BlockedBy           []string    `json:"blocked_by"`
	SourceRefs          SourceRefs  `json:"source_refs"`
	Summary             string      `json:"summary"`
	AcceptanceCriteria  []string    `json:"acceptance_criteria"`
	Constraints         []string    `json:"constraints"`
	SuggestedProfile    string      `json:"suggested_profile"`
	EstimatedComplexity int         `json:"estimated_complexity"`
	RiskLevel           string      `json:"risk_level"`
	HandoffNotes        string      `json:"handoff_notes"`
	AssignedTo          string      `json:"assigned_to,omitempty"`
}

type SourceRefs struct {
	DirectiveSections []string `json:"directive_sections"`
	UserAnswers       []string `json:"user_answers"`
}

type TestResult struct {
	Name    string `json:"name"`
	Command string `json:"command,omitempty"`
	Result  string `json:"result"`
	Output  string `json:"output,omitempty"`
}

type ToolCall struct {
	Name   string `json:"name"`
	Target string `json:"target,omitempty"`
	Status string `json:"status,omitempty"`
}

type ImplementerReport struct {
	SessionID          string            `json:"session_id"`
	TicketID           string            `json:"ticket_id"`
	Attempt            int               `json:"attempt"`
	ImplementerID      string            `json:"implementer_id"`
	StartedAt          time.Time         `json:"started_at"`
	CompletedAt        time.Time         `json:"completed_at"`
	Status             ReportStatus      `json:"status"`
	Summary            string            `json:"summary"`
	FilesChanged       []string          `json:"files_changed"`
	Commits            []string          `json:"commits"`
	GitDiffRef         string            `json:"git_diff_ref,omitempty"`
	GitDiffStat        string            `json:"git_diff_stat,omitempty"`
	TestsRun           []TestResult      `json:"tests_run"`
	ToolCalls          []ToolCall        `json:"tool_calls"`
	FilePreviews       []FileDiffPreview `json:"file_previews,omitempty"`
	ImportantDecisions []string          `json:"important_decisions"`
	KnownRisks         []string          `json:"known_risks"`
	RemainingWork      []string          `json:"remaining_work"`
	SuggestedNextStep  string            `json:"suggested_next_step"`
}

type ImplementerRunRequest struct {
	Session       SessionFile
	Ticket        TicketFile
	Implementer   AgentSpec
	Attempt       int
	WorkspacePath string
	RepoPath      string
	AgentMemory   string
	Remember      MemoryFunc
	Activity      ActivityFunc
}

type ImplementerRunResult struct {
	Status             ReportStatus
	Summary            string
	FilesChanged       []string
	Commits            []string
	GitDiffRef         string
	GitDiffStat        string
	TestsRun           []TestResult
	ToolCalls          []ToolCall
	FilePreviews       []FileDiffPreview
	ImportantDecisions []string
	KnownRisks         []string
	RemainingWork      []string
	SuggestedNextStep  string
}

type DiffLine struct {
	Type   string `json:"type"` // "context" | "add" | "remove"
	OldNum int    `json:"oldNum,omitempty"`
	NewNum int    `json:"newNum,omitempty"`
	Text   string `json:"text"`
}

type FileDiffPreview struct {
	Op         string     `json:"op"` // "edit" | "write" | "delete"
	Path       string     `json:"path"`
	AbsPath    string     `json:"absPath"`
	Lines      []DiffLine `json:"lines"`
	Truncated  bool       `json:"truncated"`
	HiddenMore int        `json:"hiddenMore,omitempty"`
	LineStart  int        `json:"lineStart,omitempty"`
	LineEnd    int        `json:"lineEnd,omitempty"`
	Adds       int        `json:"adds"`
	Removes    int        `json:"removes"`
}

type ImplementerRunner interface {
	RunImplementer(ctx context.Context, req ImplementerRunRequest) (ImplementerRunResult, error)
}

type EventStatus string

const (
	EventRunning EventStatus = "running"
	EventDone    EventStatus = "done"
	EventWaiting EventStatus = "waiting"
)

type BacklogPlanRequest struct {
	Session           SessionFile
	CM                AgentSpec
	WorkspacePath     string
	Directive         DirectiveDocument
	ImplementerAgents []AgentSpec
	AgentMemory       string
	Remember          MemoryFunc
	Activity          ActivityFunc
	Gate              func(context.Context) error // checked between exploration rounds; may be nil
}

type TicketBlueprint struct {
	Ref                 string
	Title               string
	Type                string
	Area                string
	ScopeClass          string
	Priority            string
	Summary             string
	AcceptanceCriteria  []string
	Constraints         []string
	SuggestedProfile    string
	EstimatedComplexity int
	RiskLevel           string
	HandoffNotes        string
	DependsOnRefs       []string
	SourceRefs          SourceRefs
}

type BacklogPlanResult struct {
	Summary string
	Tickets []TicketBlueprint
}

type CMQuestionGateRequest struct {
	Session       SessionFile
	CM            AgentSpec
	WorkspacePath string
	AgentMemory   string
	Remember      MemoryFunc
	Activity      ActivityFunc
	Gate          func(context.Context) error // checked between exploration rounds; may be nil
}

type CMPlanRequest struct {
	Session       SessionFile
	CM            AgentSpec
	WorkspacePath string
	UserMessage   string
	History       []ConversationEntry
	AgentMemory   string
	Remember      MemoryFunc
	Activity      ActivityFunc
	Gate          func(context.Context) error // checked between exploration rounds; may be nil
}

type CMPlanResult struct {
	Summary string
	Answer  string
}

type CMQuestionGateResult struct {
	Needed          bool
	Summary         string
	Questions       []QuestionItem
	LockedDecisions []string
	Assumptions     []string
}

type CMDirectiveRequest struct {
	Session           SessionFile
	CM                AgentSpec
	PreviousCMID      string
	WorkspacePath     string
	Version           int
	Phase             string
	ExistingDirective DirectiveDocument
	AgentMemory       string
	Remember          MemoryFunc
	Activity          ActivityFunc
	Gate              func(context.Context) error // checked between exploration rounds; may be nil
}

type CMDirectiveResult struct {
	Summary  string
	Sections map[string][]string
}

type CMReviewRequest struct {
	Session       SessionFile
	CM            AgentSpec
	WorkspacePath string
	Ticket        TicketFile
	Report        ImplementerReport
	Backlog       BacklogFile
	AutoCommit    bool
	AgentMemory   string
	Remember      MemoryFunc
	Activity      ActivityFunc
}

type CMReviewResult struct {
	Summary       string
	Decision      ReviewDecision
	Reason        string
	FollowupNotes []string
	ChildTickets  []TicketBlueprint
}

type FinalizerRequest struct {
	Session       SessionFile
	WorkspacePath string
	Backlog       BacklogFile
	Tickets       []TicketFile
	AutoCommit    AutoCommitOutcome
	AgentMemory   string
	Remember      MemoryFunc
	Activity      ActivityFunc
}

type AutoCommitOutcome struct {
	Enabled   bool
	Performed bool
	CommitSHA string
	Branch    string
	Message   string
	Error     string
}

// SessionCommitFunc commits all working-tree changes in repoPath with the
// given message and returns the new HEAD SHA. If there is nothing to commit
// it must return ("", nil). Provided by main.go so engine stays independent of
// the gitops package.
type SessionCommitFunc func(repoPath, message string) (string, error)

type FinalizerResult struct {
	Summary    string
	ReportBody string
}

type CMRunner interface {
	RunPlan(ctx context.Context, req CMPlanRequest) (CMPlanResult, error)
	RunQuestionGate(ctx context.Context, req CMQuestionGateRequest) (CMQuestionGateResult, error)
	RunDirectivePass(ctx context.Context, req CMDirectiveRequest) (CMDirectiveResult, error)
	RunBacklogPlan(ctx context.Context, req BacklogPlanRequest) (BacklogPlanResult, error)
	RunReview(ctx context.Context, req CMReviewRequest) (CMReviewResult, error)
	RunFinalizer(ctx context.Context, req FinalizerRequest) (FinalizerResult, error)
}

type TestRunRequest struct {
	RepoPath      string
	WorkspacePath string
	Session       SessionFile
	Ticket        TicketFile
	ChangedFiles  []string
}

type TestRunner interface {
	Run(ctx context.Context, req TestRunRequest) ([]TestResult, error)
}

type ReviewNextAction struct {
	Type          string  `json:"type"`
	TicketID      *string `json:"ticket_id,omitempty"`
	ImplementerID *string `json:"implementer_id,omitempty"`
}

type ReviewLogEntry struct {
	Timestamp     time.Time        `json:"timestamp"`
	ReviewerID    string           `json:"reviewer_id"`
	TicketID      string           `json:"ticket_id"`
	Attempt       int              `json:"attempt"`
	Decision      ReviewDecision   `json:"decision"`
	Reason        string           `json:"reason"`
	GitDiffRef    string           `json:"git_diff_ref,omitempty"`
	GitDiffStat   string           `json:"git_diff_stat,omitempty"`
	NextAction    ReviewNextAction `json:"next_action"`
	FollowupNotes []string         `json:"followup_notes"`
}

type BacklogFile struct {
	SessionID     string            `json:"session_id"`
	GeneratedBy   string            `json:"generated_by"`
	GeneratedAt   time.Time         `json:"generated_at"`
	Frozen        bool              `json:"frozen"`
	ScopeSummary  ScopeSummary      `json:"scope_summary"`
	TicketOrder   []string          `json:"ticket_order"`
	ReadyQueue    []string          `json:"ready_queue"`
	BlockedQueue  []string          `json:"blocked_queue"`
	DeferredQueue []string          `json:"deferred_queue"`
	DoneQueue     []string          `json:"done_queue"`
	TicketIndex   map[string]string `json:"ticket_index"`
}

type BootstrapRequest struct {
	UserRequest     string
	UserRepoPath    string
	ConfigSnapshot  any
	RepoSnapshot    any
	Mode            SessionMode
	CMCount         int
	ImplementerMax  int
	EnableFinalizer bool
	AutoCommit      bool
}

type BootstrapResult struct {
	Session       SessionFile
	Roster        RosterFile
	Backlog       BacklogFile
	WorkspacePath string
}

type ConversationEntry struct {
	Timestamp time.Time   `json:"timestamp"`
	Mode      SessionMode `json:"mode"`
	Role      string      `json:"role"`
	ActorID   string      `json:"actor_id,omitempty"`
	Content   string      `json:"content"`
}

type Event struct {
	ID          string
	Role        string
	ActorID     string
	Phase       string
	TicketID    string
	Status      EventStatus
	Content     string
	TicketIndex int    // 1-based position of TicketID in the dispatch order (0 if N/A)
	TicketTotal int    // total dispatchable tickets in the backlog (0 if N/A)
	TicketTitle string // human-readable ticket title
}

// Activity is a fine-grained, real-time signal of what an agent is doing
// right now (e.g. which tool it called and against which file). It is
// orthogonal to Event — Events drive the durable message stream, Activities
// drive the live "what is the model doing" indicator. Activities are not
// persisted; they are best-effort telemetry for the UI.
type Activity struct {
	Kind     string `json:"kind"`     // "tool_start" | "tool_done" | "tool_failed" | "thinking" | "phase"
	Actor    string `json:"actor"`    // e.g. "cm-01", "impl-02", "finalizer-01"
	Role     string `json:"role"`     // "contre-maitre" | "implementer" | "finalizer" | "system"
	Phase    string `json:"phase"`    // free-form phase label (e.g. "implementing", "directive pass")
	TicketID string `json:"ticketId"` // active ticket if any
	Tool     string `json:"tool"`     // tool name (file_read, grep, glob, list, file_edit, file_write, file_delete)
	Target   string `json:"target"`   // primary tool target (file path / pattern / pattern)
	Detail   string `json:"detail"`   // pre-formatted human caption (e.g. "Reading main.go L1-200")
	Status   string `json:"status"`   // "running" | "done" | "failed"
}

// ActivityFunc receives a single Activity. Implementations must be safe to
// call from the runner goroutine. Engine wraps the user-supplied callback to
// stamp Actor/Role/Phase/TicketID context onto each event.
type ActivityFunc func(Activity)

type MemoryEvent struct {
	Time     time.Time      `json:"time"`
	AgentID  string         `json:"agent_id"`
	Kind     string         `json:"kind"`
	Phase    string         `json:"phase,omitempty"`
	TicketID string         `json:"ticket_id,omitempty"`
	Content  string         `json:"content"`
	Meta     map[string]any `json:"meta,omitempty"`
}

type MemoryFunc func(MemoryEvent)
