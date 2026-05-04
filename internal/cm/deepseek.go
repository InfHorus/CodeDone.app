package cm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codedone/internal/engine"
	"codedone/internal/gitops"
	"codedone/internal/guidance"
	anthropicprovider "codedone/internal/provider/anthropic"
	chatprovider "codedone/internal/provider/chat"
	deepseekprovider "codedone/internal/provider/deepseek"
	openaiprovider "codedone/internal/provider/openai"
	openrouterprovider "codedone/internal/provider/openrouter"
	"codedone/internal/repotools"
	"codedone/internal/shellexec"
)

type DeepSeekRunner struct {
	client      chatClient
	git         *gitops.Service
	workDir     string
	model       string
	maxTokens   int
	temperature float64
}

type chatClient interface {
	ChatCompletion(ctx context.Context, req chatprovider.ChatRequest) (*chatprovider.ChatResponse, error)
}

func NewDeepSeekRunner(apiKey string, git *gitops.Service, workDir, model string, maxTokens int, temperature float64, timeout time.Duration) *DeepSeekRunner {
	return newDeepSeekCompatibleRunner(deepseekprovider.NewClient(apiKey, timeout), git, workDir, model, maxTokens, temperature)
}

func NewOpenRouterRunner(apiKey string, git *gitops.Service, workDir, model string, maxTokens int, temperature float64, timeout time.Duration) *DeepSeekRunner {
	return newDeepSeekCompatibleRunner(openrouterprovider.NewClient(apiKey, timeout), git, workDir, model, maxTokens, temperature)
}

func NewOpenAIRunner(apiKey string, git *gitops.Service, workDir, model string, maxTokens int, temperature float64, timeout time.Duration) *DeepSeekRunner {
	return newDeepSeekCompatibleRunner(openaiprovider.NewClient(apiKey, timeout), git, workDir, model, maxTokens, temperature)
}

func NewAnthropicRunner(apiKey string, git *gitops.Service, workDir, model string, maxTokens int, temperature float64, timeout time.Duration) *DeepSeekRunner {
	return newDeepSeekCompatibleRunner(anthropicprovider.NewClient(apiKey, timeout), git, workDir, model, maxTokens, temperature)
}

func newDeepSeekCompatibleRunner(client chatClient, git *gitops.Service, workDir, model string, maxTokens int, temperature float64) *DeepSeekRunner {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "deepseek-v4-flash"
	}
	if maxTokens <= 0 {
		maxTokens = 32768
	}
	return &DeepSeekRunner{
		client:      client,
		git:         git,
		workDir:     workDir,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
	}
}

func (r *DeepSeekRunner) RunPlan(ctx context.Context, req engine.CMPlanRequest) (engine.CMPlanResult, error) {
	prompt, meta := r.buildPlanPrompt(req)
	systemPrompt := "You are the single Contre-Maitre in CodeDone Plan mode. Talk normally with the user. You may inspect the repository with browse-only tools (list, glob, grep, file_read) and relevant guidance tools (guidance_list, guidance_get) before answering. Only use the shell tool when something cannot be answered by list/glob/grep/file_read — for example, to run npm ls or read a file outside the repo. Check the repo summary in the user message for the current HEAD; if it says NONE or greenfield, do NOT run any git commands. If Agent Memory already contains the relevant guidance category, use that remembered guidance instead of fetching it again unless you have a specific reason to refresh it. Do not create tickets, do not dispatch implementers, do not write files, do not claim implementation is complete. Be direct, concrete, and ground codebase answers in files you inspected. Treat guidance as a quality bar, not extra scope. Final answer should be normal prose or concise markdown, not JSON."
	content, rounds, toolCalls, lastResp, err := r.chatWithExploration(ctx, req.Session.UserRepoPath, systemPrompt, prompt, req.AgentMemory, req.CM.ID, "plan", "", req.Remember, req.Activity, req.Gate)
	if err != nil {
		return engine.CMPlanResult{}, err
	}
	respID, respModel := "", r.model
	if lastResp != nil {
		respID = lastResp.ID
		respModel = lastResp.Model
	}
	r.writeRawResponse(req.WorkspacePath, fmt.Sprintf("%s.plan.deepseek.json", req.CM.ID), map[string]any{
		"cm_id":       req.CM.ID,
		"model":       respModel,
		"response_id": respID,
		"prompt_meta": meta,
		"rounds":      rounds,
		"tool_calls":  toolCalls,
		"raw_content": content,
	})
	answer := strings.TrimSpace(content)
	return engine.CMPlanResult{
		Summary: summarizePlanAnswer(answer),
		Answer:  answer,
	}, nil
}

func (r *DeepSeekRunner) RunQuestionGate(ctx context.Context, req engine.CMQuestionGateRequest) (engine.CMQuestionGateResult, error) {
	prompt, meta := r.buildQuestionGatePrompt(req)
	systemPrompt := "You are Contre-Maitre 1 for CodeDone. Decide whether the user must be asked clarifying questions before autonomous execution. Before answering, USE THE BROWSE TOOLS (list, glob, grep, file_read) to inspect the user repo so your decision is grounded in the actual current code. Only use the shell tool when something cannot be answered by list/glob/grep/file_read. Check the repo summary in the user message for the current HEAD; if it says NONE or greenfield, do NOT run any git commands — they will fail on an empty repo. If the request clearly matches a guidance category, use guidance_list and guidance_get for that category unless Agent Memory already contains the relevant guidance. Ask at most 5 questions and only if the answers would change architecture, scope, irreversible product decisions, or execution strategy. When you are done exploring, return STRICT JSON only as the final assistant message — no prose, no code fences, no tool calls."
	content, rounds, toolCalls, lastResp, err := r.chatWithExploration(ctx, req.Session.UserRepoPath, systemPrompt, prompt, req.AgentMemory, req.CM.ID, "question_gate", "", req.Remember, req.Activity, req.Gate)
	if err != nil {
		return engine.CMQuestionGateResult{}, err
	}
	respID, respModel := "", r.model
	if lastResp != nil {
		respID = lastResp.ID
		respModel = lastResp.Model
	}
	r.writeRawResponse(req.WorkspacePath, fmt.Sprintf("%s.question-gate.deepseek.json", req.CM.ID), map[string]any{
		"cm_id":       req.CM.ID,
		"model":       respModel,
		"response_id": respID,
		"prompt_meta": meta,
		"rounds":      rounds,
		"tool_calls":  toolCalls,
		"raw_content": content,
	})

	parsed := parseQuestionGateResponse(content)
	questions := normalizeQuestions(parsed.Questions, req.Session.QuestionGate.MaxQuestions)
	locked := normalizeLines(parsed.LockedDecisions)
	assumptions := normalizeLines(parsed.Assumptions)
	needed := parsed.Needed && len(questions) > 0
	if !needed {
		questions = nil
	}

	summary := strings.TrimSpace(parsed.Summary)
	if summary == "" {
		if needed {
			summary = fmt.Sprintf("%s decided that user clarification is required before planning can continue.", req.CM.ID)
		} else {
			summary = fmt.Sprintf("%s determined the request is clear enough to proceed without user questions.", req.CM.ID)
		}
	}

	return engine.CMQuestionGateResult{
		Needed:          needed,
		Summary:         summary,
		Questions:       questions,
		LockedDecisions: locked,
		Assumptions:     assumptions,
	}, nil
}

func (r *DeepSeekRunner) RunDirectivePass(ctx context.Context, req engine.CMDirectiveRequest) (engine.CMDirectiveResult, error) {
	prompt, meta := r.buildDirectivePrompt(req)
	systemPrompt := "You are a Contre-Maitre agent for CodeDone. Produce a rigorous directive pass for a serialized coding session. Before drafting, USE THE BROWSE TOOLS (list, glob, grep, file_read) to inspect the user repo and ground every claim in the actual current code — do not assume the repo is empty. Only use the shell tool when something cannot be answered by list/glob/grep/file_read. Check the repo summary in the user message for the current HEAD; if it says NONE or greenfield, do NOT run any git commands — they will fail on an empty repo. If the request clearly matches a guidance category and is non-trivial, use guidance_list and guidance_get for that category unless Agent Memory already contains the relevant guidance; for very basic tasks or obvious small edits, skip guidance and rely on repo inspection. Expand underspecified product requirements aggressively when the user's wording implies breadth, examples, 'etc', or an open-ended product ask; keep explicit narrow/basic requests narrow. Treat guidance as a quality bar, not extra scope. When you are done exploring, return STRICT JSON only as the final assistant message — no prose, no code fences, no tool calls."
	content, rounds, toolCalls, lastResp, err := r.chatWithExploration(ctx, req.Session.UserRepoPath, systemPrompt, prompt, req.AgentMemory, req.CM.ID, req.Phase, "", req.Remember, req.Activity, req.Gate)
	if err != nil {
		return engine.CMDirectiveResult{}, err
	}
	respID, respModel := "", r.model
	if lastResp != nil {
		respID = lastResp.ID
		respModel = lastResp.Model
	}
	r.writeRawResponse(req.WorkspacePath, fmt.Sprintf("%s.directive-v%d.deepseek.json", req.CM.ID, req.Version), map[string]any{
		"cm_id":       req.CM.ID,
		"phase":       req.Phase,
		"version":     req.Version,
		"model":       respModel,
		"response_id": respID,
		"prompt_meta": meta,
		"rounds":      rounds,
		"tool_calls":  toolCalls,
		"raw_content": content,
	})

	parsed := parseDirectiveResponse(content)
	sections := map[string][]string{
		"session_identity":            normalizeLines(parsed.SessionIdentity),
		"user_request":                normalizeLines(parsed.UserRequest),
		"locked_decisions":            normalizeLines(parsed.LockedDecisions),
		"assumptions":                 normalizeLines(parsed.Assumptions),
		"product_framing":             normalizeLines(parsed.ProductFraming),
		"competitive":                 normalizeLines(parsed.Competitive),
		"scope_required":              normalizeLines(parsed.ScopeRequired),
		"scope_best":                  normalizeLines(parsed.ScopeBest),
		"scope_stretch":               normalizeLines(parsed.ScopeStretch),
		"architecture_direction":      normalizeLines(parsed.ArchitectureDirection),
		"full_feature_inventory":      normalizeLines(parsed.FullFeatureInventory),
		"risks_and_unknowns":          normalizeLines(parsed.RisksAndUnknowns),
		"backlog_strategy":            normalizeLines(parsed.BacklogStrategy),
		"dispatch_notes_for_final_cm": normalizeLines(parsed.DispatchNotesForFinalCM),
	}
	summary := strings.TrimSpace(parsed.Summary)
	if summary == "" {
		summary = fmt.Sprintf("%s completed directive pass v%d.", req.CM.ID, req.Version)
	}

	return engine.CMDirectiveResult{
		Summary:  summary,
		Sections: sections,
	}, nil
}

func (r *DeepSeekRunner) buildQuestionGatePrompt(req engine.CMQuestionGateRequest) (string, map[string]any) {
	repoSummary, fileManifest := r.repoContext(req.Session.UserRepoPath)
	planContext := engine.PlanContextForPrompt(req.WorkspacePath)
	prompt := strings.Join([]string{
		"Return strict JSON only with this shape:",
		`{"needed":true,"summary":"...","questions":[{"id":"Q1","prompt":"...","rationale":"..."}],"locked_decisions":["..."],"assumptions":["..."]}`,
		"",
		fmt.Sprintf("Session ID: %s", req.Session.SessionID),
		fmt.Sprintf("Contre-Maitre: %s", req.CM.ID),
		fmt.Sprintf("User request: %s", req.Session.UserRequest),
		"",
		"Prior plan-mode context:",
		planContext,
		"",
		"Rules:",
		"- This is the first step of the session.",
		"- Before deciding, USE the browse tools (list, glob, grep, file_read) to inspect the user repo. Start with list on '.', then read the obviously relevant files (e.g., index.html, README, main.go, package.json) so your decision is grounded in what actually exists.",
		"- Ask questions only if they matter materially.",
		"- Ask at most 5 questions.",
		"- If the request is already actionable, set needed=false and return questions=[].",
		"- If you can proceed using explicit assumptions, prefer that over asking low-value questions.",
		"",
		"Repo summary:",
		repoSummary,
		"",
		"Repo file manifest sample:",
		fileManifest,
	}, "\n")

	return prompt, map[string]any{
		"session_id":   req.Session.SessionID,
		"cm_id":        req.CM.ID,
		"manifest_len": len(strings.Split(fileManifest, "\n")),
	}
}

func (r *DeepSeekRunner) buildPlanPrompt(req engine.CMPlanRequest) (string, map[string]any) {
	repoSummary, fileManifest := r.repoContext(req.Session.UserRepoPath)
	historyLines := make([]string, 0, len(req.History))
	for _, item := range req.History {
		actor := item.Role
		if strings.TrimSpace(item.ActorID) != "" {
			actor += "/" + item.ActorID
		}
		historyLines = append(historyLines, fmt.Sprintf("- %s: %s", actor, limitPromptLine(item.Content, 1200)))
	}
	if len(historyLines) == 0 {
		historyLines = append(historyLines, "- No prior plan conversation.")
	}
	prompt := strings.Join([]string{
		fmt.Sprintf("Session ID: %s", req.Session.SessionID),
		fmt.Sprintf("Contre-Maitre: %s", req.CM.ID),
		fmt.Sprintf("Original user goal: %s", req.Session.UserRequest),
		"",
		"Current user message:",
		req.UserMessage,
		"",
		"Plan-mode conversation so far:",
		strings.Join(historyLines, "\n"),
		"",
		"Repo summary:",
		repoSummary,
		"",
		"Repo file manifest sample:",
		fileManifest,
		"",
		"Instructions:",
		"- Answer the user's current message directly.",
		"- Use browse tools when repo context would materially improve the answer.",
		"- If you cite code, mention the file paths you inspected.",
		"- You are in Plan mode: do not make tickets, do not dispatch implementers, and do not promise file changes.",
		"- If the user seems ready to execute, describe what Build mode would do next but wait for the user to switch/send in Build.",
	}, "\n")
	return prompt, map[string]any{
		"session_id":    req.Session.SessionID,
		"cm_id":         req.CM.ID,
		"history_items": len(req.History),
		"manifest_len":  len(strings.Split(fileManifest, "\n")),
	}
}

func (r *DeepSeekRunner) RunBacklogPlan(ctx context.Context, req engine.BacklogPlanRequest) (engine.BacklogPlanResult, error) {
	prompt, meta := r.buildBacklogPrompt(req)
	systemPrompt := "You are the final Contre-Maitre for CodeDone. Convert the frozen directive into a strong serialized backlog of implementable tickets. The directive already contains the full repo analysis — do NOT re-inspect the repo or run git commands; use the directive as your sole source of truth about the codebase. You may use list/glob/grep/file_read only to confirm the existence of a specific file when the directive references it and you need its exact current content. If the work clearly matches a guidance category and is non-trivial, use guidance_list and guidance_get for that category unless Agent Memory already contains the relevant guidance; for very basic tasks or obvious small edits, skip guidance. Treat guidance as a quality bar, not extra scope. Size tickets intelligently: group tightly-coupled simple work into one ticket, split only when implementation/review risk or dependencies justify it. When you are done, return STRICT JSON only as the final assistant message — no prose, no code fences, no tool calls."
	content, rounds, toolCalls, lastResp, err := r.chatWithExploration(ctx, req.Session.UserRepoPath, systemPrompt, prompt, req.AgentMemory, req.CM.ID, "backlog", "", req.Remember, req.Activity, req.Gate)
	if err != nil {
		return engine.BacklogPlanResult{}, err
	}
	respID, respModel := "", r.model
	if lastResp != nil {
		respID = lastResp.ID
		respModel = lastResp.Model
	}
	r.writeRawResponse(req.WorkspacePath, fmt.Sprintf("%s.backlog-plan.deepseek.json", req.CM.ID), map[string]any{
		"cm_id":       req.CM.ID,
		"model":       respModel,
		"response_id": respID,
		"prompt_meta": meta,
		"rounds":      rounds,
		"tool_calls":  toolCalls,
		"raw_content": content,
	})

	parsed := parseBacklogResponse(content)
	return engine.BacklogPlanResult{
		Summary: strings.TrimSpace(parsed.Summary),
		Tickets: normalizeBacklogBlueprints(parsed.Tickets),
	}, nil
}

func (r *DeepSeekRunner) RunReview(ctx context.Context, req engine.CMReviewRequest) (engine.CMReviewResult, error) {
	prompt, meta := r.buildReviewPrompt(req)
	emitCMActivity(req.Activity, engine.Activity{
		Kind:   "thinking",
		Status: "running",
		Detail: fmt.Sprintf("Reviewing %s", req.Ticket.ID),
	})
	resp, err := r.client.ChatCompletion(ctx, chatprovider.ChatRequest{
		Model: r.model,
		Messages: []chatprovider.Message{
			{
				Role:    "system",
				Content: "You are the final Contre-Maitre for CodeDone. Review one implementer attempt using the ticket, report, ticket-scoped diff previews, changed file contents, and real test results. The implementer is NEVER responsible for git: commits, staging, and pushes are owned by the engine and decided once at session end based on the user's auto-commit setting. Do not request a revision because no commit happened, because a bash/git tool call failed, or because tools outside the implementer's allowed list were attempted — judge purely on the file changes against the ticket criteria. Return strict JSON only. Prefer acceptance only when the ticket is actually complete.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Thinking:    &chatprovider.ThinkingConfig{Type: "disabled"},
		Stream:      false,
		MaxTokens:   r.maxTokens,
		Temperature: r.temperature,
	})
	if err != nil {
		return engine.CMReviewResult{}, err
	}

	content := extractContent(resp)
	r.writeRawResponse(req.WorkspacePath, fmt.Sprintf("%s.review-%s.attempt-%d.deepseek.json", req.CM.ID, req.Ticket.ID, req.Report.Attempt), map[string]any{
		"cm_id":       req.CM.ID,
		"ticket_id":   req.Ticket.ID,
		"attempt":     req.Report.Attempt,
		"model":       resp.Model,
		"response_id": resp.ID,
		"prompt_meta": meta,
		"raw_content": content,
		"usage":       resp.Usage,
		"choices":     resp.Choices,
	})

	parsed := parseReviewResponse(content)
	decision := parsed.Decision
	if decision == "" {
		decision = string(engine.DecisionAccept)
	}
	return engine.CMReviewResult{
		Summary:       strings.TrimSpace(parsed.Summary),
		Decision:      engine.ReviewDecision(decision),
		Reason:        strings.TrimSpace(parsed.Reason),
		FollowupNotes: normalizeLines(parsed.FollowupNotes),
		ChildTickets:  normalizeBacklogBlueprints(parsed.ChildTickets),
	}, nil
}

func (r *DeepSeekRunner) buildDirectivePrompt(req engine.CMDirectiveRequest) (string, map[string]any) {
	repoSummary, fileManifest := r.repoContext(req.Session.UserRepoPath)
	planContext := engine.PlanContextForPrompt(req.WorkspacePath)
	existingDirective := "(none)"
	if req.ExistingDirective.SessionID != "" {
		if data, err := os.ReadFile(filepath.Join(req.WorkspacePath, "directive.md")); err == nil {
			existingDirective = string(data)
			if len(existingDirective) > 12000 {
				existingDirective = existingDirective[:12000]
			}
		}
	}
	questionSummary := formatQuestionBatch(req.Session.QuestionBatch)
	stageInstruction := directiveStageInstruction(req)

	prompt := strings.Join([]string{
		"Return strict JSON only with this shape:",
		`{"summary":"...","session_identity":["..."],"user_request":["..."],"locked_decisions":["..."],"assumptions":["..."],"product_framing":["..."],"competitive":["..."],"scope_required":["..."],"scope_best":["..."],"scope_stretch":["..."],"architecture_direction":["..."],"full_feature_inventory":["..."],"risks_and_unknowns":["..."],"backlog_strategy":["..."],"dispatch_notes_for_final_cm":["..."]}`,
		"",
		fmt.Sprintf("Session ID: %s", req.Session.SessionID),
		fmt.Sprintf("Current CM: %s", req.CM.ID),
		fmt.Sprintf("Previous CM: %s", req.PreviousCMID),
		fmt.Sprintf("Directive version: %d", req.Version),
		fmt.Sprintf("Phase: %s", req.Phase),
		fmt.Sprintf("User request: %s", req.Session.UserRequest),
		"",
		"Prior plan-mode context:",
		planContext,
		"",
		"Question gate:",
		questionSummary,
		"",
		"Repo summary:",
		repoSummary,
		"",
		"Repo file manifest sample:",
		fileManifest,
		"",
		"Existing directive:",
		existingDirective,
		"",
		"Directive rules:",
		"- Before drafting, use the browse tools (list, glob, grep, file_read) to inspect the user repo and ground every claim in the actual current code. Start with list on '.', then read the obviously relevant files for the request.",
		"- Use guidance only when it is likely to change the plan quality. For very basic tasks, tiny edits, or obvious single-file work, skip guidance unless the user explicitly asks for design/security/framework depth.",
		"- Use short, concrete bullet points that are directly useful for planning and dispatch.",
		"- FIRST classify the request scope before drafting:",
		"  TYPE A — Open-ended product/app/tool: user names a concept with little implementation detail ('build a web OS', 'make a todo app', 'create a dashboard'). Signals: product nouns, vague verbs (build/make/create), few constraints, open scope.",
		"  TYPE B — Targeted change: user specifies a concrete action on existing code ('fix this bug', 'add dark mode toggle', 'refactor auth'). Signals: references existing code/files/components, explicitly scoped ('just', 'only', 'small', 'quick').",
		"- For TYPE A: enumerate the complete realistic feature set for that product category. Think like a product manager — what screens, interactions, data flows, and system behaviors does a real version of this product have? List every feature the user would obviously expect even if unmentioned (e.g. 'web OS' implies desktop, windows, taskbar, right-click context menu, filesystem, app launcher, drag & drop, resize/minimize/maximize, wallpaper, etc.). Do NOT hold back to match the literal words — the user's brevity is an invitation to think for them. Categorize as required / best-in-class / stretch.",
		"- For TYPE B: stay strictly within the stated scope. Do not introduce persistence, extra menus, extra applications, or any feature not asked for. A fix is a fix; an addition is exactly what was asked and nothing more.",
		"- Explicitly separate what already exists from what must be built or improved, so later tickets do not redo accepted work.",
		"- If the user explicitly says 'basic', 'minimal', 'simple', 'demo', or 'test build', treat it as TYPE B regardless of phrasing.",
		"- Distinguish required, best-in-class, and stretch scope clearly.",
		"- Keep the system serialized: one active implementer ticket at a time.",
		"- Do not ask new questions here. Work from the available request, repo, and answers.",
		"",
		"Stage-specific instruction:",
		stageInstruction,
	}, "\n")

	return prompt, map[string]any{
		"session_id":   req.Session.SessionID,
		"cm_id":        req.CM.ID,
		"phase":        req.Phase,
		"version":      req.Version,
		"manifest_len": len(strings.Split(fileManifest, "\n")),
	}
}

func (r *DeepSeekRunner) RunFinalizer(ctx context.Context, req engine.FinalizerRequest) (engine.FinalizerResult, error) {
	prompt, meta := r.buildFinalizerPrompt(req)
	emitCMActivity(req.Activity, engine.Activity{
		Kind:   "thinking",
		Status: "running",
		Detail: "Writing final report",
	})
	resp, err := r.client.ChatCompletion(ctx, chatprovider.ChatRequest{
		Model: r.model,
		Messages: []chatprovider.Message{
			{
				Role:    "system",
				Content: "You are the Finalizer agent for CodeDone. Write a concise final validation report for the completed serialized session. Return strict JSON only.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Thinking:    &chatprovider.ThinkingConfig{Type: "disabled"},
		Stream:      false,
		MaxTokens:   r.maxTokens,
		Temperature: r.temperature,
	})
	if err != nil {
		return engine.FinalizerResult{}, err
	}

	content := extractContent(resp)
	r.writeRawResponse(req.WorkspacePath, "finalizer.deepseek.json", map[string]any{
		"model":       resp.Model,
		"response_id": resp.ID,
		"prompt_meta": meta,
		"raw_content": content,
		"usage":       resp.Usage,
		"choices":     resp.Choices,
	})
	parsed := parseFinalizerResponse(content)
	return engine.FinalizerResult{
		Summary:    strings.TrimSpace(parsed.Summary),
		ReportBody: strings.TrimSpace(parsed.ReportBody),
	}, nil
}

func (r *DeepSeekRunner) buildBacklogPrompt(req engine.BacklogPlanRequest) (string, map[string]any) {
	planContext := engine.PlanContextForPrompt(req.WorkspacePath)
	repoSummary, _ := r.repoContext(req.Session.UserRepoPath)
	directiveText := "(directive unavailable)"
	if data, err := os.ReadFile(filepath.Join(req.WorkspacePath, "directive.md")); err == nil {
		directiveText = string(data)
		if len(directiveText) > 18000 {
			directiveText = directiveText[:18000]
		}
	}
	rosterLines := make([]string, 0, len(req.ImplementerAgents))
	for _, agent := range req.ImplementerAgents {
		rosterLines = append(rosterLines, fmt.Sprintf("- %s | profile=%s | specialties=%s", agent.ID, agent.Profile, strings.Join(agent.Specialties, ",")))
	}
	prompt := strings.Join([]string{
		"Return strict JSON only with this shape:",
		`{"summary":"...","tickets":[{"ref":"P1","title":"...","type":"feature","area":"...","scope_class":"required|best_in_class|stretch","priority":"high|medium|low","summary":"...","acceptance_criteria":["..."],"constraints":["..."],"suggested_profile":"backend|frontend|tests|git|state|integrations|platform|infra|refactor|data","estimated_complexity":1,"risk_level":"low|medium|high","handoff_notes":"...","depends_on_refs":["P0"],"source_refs":{"directive_sections":["..."],"user_answers":["..."]}}]}`,
		"",
		fmt.Sprintf("Session ID: %s", req.Session.SessionID),
		fmt.Sprintf("Final CM: %s", req.CM.ID),
		fmt.Sprintf("Implementer max: %d", req.Session.ImplementerMax),
		"",
		"Repo state:",
		repoSummary,
		"",
		"Rules:",
		"- Produce tickets suitable for serialized execution.",
		"- Each ticket should fit one implementer pass and one CM review, but it does not need to be artificially tiny.",
		"- One ticket should cover one concrete slice, not the whole feature set.",
		"- Group tightly-coupled simple work into a single ticket. For example, a basic Notepad app can be one ticket including its HTML, styling, open/close behavior, typing area, and basic interactions.",
		"- Split only when the work has real dependency boundaries, separate risk, separate files/owners, or would be hard to review as one coherent change.",
		"- Write each ticket as an implementer handoff, not a vague product note: describe the target behavior, likely files/areas, existing behavior to preserve, dependencies, and concrete review evidence.",
		"- Before creating a ticket, account for what already exists in the repo and in earlier tickets. If a slice is already done, do not create a duplicate ticket; if it needs polish, phrase the ticket as an improvement over the current state.",
		"- For UI builds, split structure/layout, interaction behavior, and polish into separate tickets when that makes review clearer.",
		"- For small prototype requests, prefer the smallest backlog that can satisfy the goal cleanly. Usually 1 to 3 required tickets is enough.",
		"- Do not explode a narrow prototype into many tiny tickets unless the behavior is genuinely separable and review would be clearer.",
		"- Keep acceptance criteria tight and specific.",
		"- Acceptance criteria must include observable behavior, required wiring, and any verification expectation. Avoid criteria that only say 'looks polished' or 'works correctly'.",
		"- Use depends_on_refs to preserve logical order when needed.",
		"- Assign suggested_profile based on the work type.",
		"- Include required work first, then best-in-class, then stretch.",
		"- Do not produce placeholders or vague backlog items.",
		"- A ticket must not include acceptance criteria that really belong to later tickets.",
		"- Handoff notes should warn implementers to continue from current file state, avoid replacing accepted work, use file_edit for existing files, and call complete if inspection shows the ticket is already satisfied.",
		"",
		"Available implementer profiles:",
		strings.Join(rosterLines, "\n"),
		"",
		"Prior plan-mode context:",
		planContext,
		"",
		"Directive:",
		directiveText,
	}, "\n")
	return prompt, map[string]any{
		"session_id": req.Session.SessionID,
		"cm_id":      req.CM.ID,
		"agents":     len(req.ImplementerAgents),
	}
}

func (r *DeepSeekRunner) buildReviewPrompt(req engine.CMReviewRequest) (string, map[string]any) {
	changedFiles := buildChangedFileContexts(req.Session.UserRepoPath, req.Report.FilesChanged)
	attemptDiff := renderFilePreviews(req.Report.FilePreviews)
	reportJSON := string(mustJSONBytes(req.Report))
	if len(reportJSON) > 12000 {
		reportJSON = reportJSON[:12000]
	}
	prompt := strings.Join([]string{
		"Return strict JSON only with this shape:",
		`{"summary":"...","decision":"accept|revise|split|defer|block","reason":"...","followup_notes":["..."],"child_tickets":[{"ref":"C1","title":"...","type":"feature","area":"...","scope_class":"required","priority":"high","summary":"...","acceptance_criteria":["..."],"constraints":["..."],"suggested_profile":"backend","estimated_complexity":1,"risk_level":"medium","handoff_notes":"...","depends_on_refs":[],"source_refs":{"directive_sections":["..."],"user_answers":["..."]}}]}`,
		"",
		fmt.Sprintf("Session ID: %s", req.Session.SessionID),
		fmt.Sprintf("Reviewer: %s", req.CM.ID),
		fmt.Sprintf("Ticket ID: %s", req.Ticket.ID),
		"",
		"Agent Memory:",
		emptyIfBlank(req.AgentMemory),
		"",
		"Review rules:",
		"- Accept only if the ticket is genuinely complete and tests are not failing.",
		"- A no-change implementer report can be accepted when status=done, the report explains file-grounded evidence, and the current repo already satisfies the ticket. Do not force file churn for already-complete work.",
		"- Judge implementation quality and relevance, not just whether files changed or tool calls succeeded.",
		"- Verify the diff actually implements the ticket's user-visible behavior and acceptance criteria.",
		"- Revise if the implementation is fake, brittle, superficial, hard-coded to pass a narrow case, or unrelated to the requested behavior.",
		"- Revise if changed code is likely to regress adjacent behavior in the touched area, even when the ticket's happy path appears covered.",
		"- Revise or split when the implementation avoids the existing architecture, duplicates major logic, or leaves inconsistent state across modules.",
		"- Reject or revise scope creep when the implementer adds major features that belong to later tickets.",
		"- Revise if the ticket is close but incomplete or structurally weak.",
		"- When an attempt contains useful partial file changes but misses criteria, describe exactly what remains and what existing partial work the next implementer must preserve or continue.",
		"- Do not treat one recoverable tool-call error as proof that the whole task is impossible. Review persisted file changes and criteria first.",
		"- Split only if the ticket was too broad and should be replaced by smaller tickets.",
		"- Defer only for low-priority or stretch work.",
		"- Block only for hard blockers that require a different prerequisite or user intervention.",
		"- If decision=split, provide child_tickets.",
		"- Do not request revision solely because there is no automated test framework.",
		"- The implementer is NEVER responsible for git, commits, staging, or pushing. The engine commits once at the end of the session if and only if the user's auto-commit setting is on (see Auto-commit policy below). Do NOT revise/block because a commit is missing, because a bash/git tool call failed, or because the implementer did not stage anything — judge purely on the file changes against the ticket criteria.",
		"- If the changed file excerpts and report concretely cover the ticket criteria, decide from that evidence instead of looping for perfect proof.",
		"- Treat files_changed and ticket-scoped diff previews as the only files changed by this attempt. Do not infer scope creep from unrelated dirty working-tree state.",
		"",
		fmt.Sprintf("Auto-commit policy: enabled=%t (engine handles end-of-session commit; the implementer never commits).", req.AutoCommit),
		"",
		"Ticket:",
		string(mustJSONBytes(req.Ticket)),
		"",
		"Implementer report:",
		reportJSON,
		"",
		"Ticket-scoped diff previews:",
		attemptDiff,
		"",
		"Changed file excerpts:",
		strings.Join(changedFiles, "\n\n---\n\n"),
	}, "\n")
	return prompt, map[string]any{
		"session_id":  req.Session.SessionID,
		"ticket_id":   req.Ticket.ID,
		"attempt":     req.Report.Attempt,
		"changed_len": len(req.Report.FilesChanged),
	}
}

func renderFilePreviews(previews []engine.FileDiffPreview) string {
	if len(previews) == 0 {
		return "(ticket-scoped diff unavailable; use implementer report and changed file excerpts only)"
	}
	parts := make([]string, 0, len(previews))
	for _, preview := range previews {
		lines := []string{
			fmt.Sprintf("File: %s", preview.Path),
			fmt.Sprintf("Operation: %s (+%d/-%d)", preview.Op, preview.Adds, preview.Removes),
		}
		for _, line := range preview.Lines {
			prefix := " "
			switch line.Type {
			case "add":
				prefix = "+"
			case "remove":
				prefix = "-"
			}
			lines = append(lines, prefix+line.Text)
		}
		if preview.Truncated {
			lines = append(lines, fmt.Sprintf("... truncated %d additional line(s)", preview.HiddenMore))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	out := strings.Join(parts, "\n\n---\n\n")
	if len(out) > 24000 {
		out = out[:24000]
	}
	return out
}

func (r *DeepSeekRunner) buildFinalizerPrompt(req engine.FinalizerRequest) (string, map[string]any) {
	reviewLog := ""
	if data, err := os.ReadFile(filepath.Join(req.WorkspacePath, "review-log.jsonl")); err == nil {
		reviewLog = string(data)
		if len(reviewLog) > 12000 {
			reviewLog = reviewLog[:12000]
		}
	}
	reportNames := make([]string, 0, len(req.Tickets))
	ticketStates := make([]map[string]any, 0, len(req.Tickets))
	for _, ticket := range req.Tickets {
		reportNames = append(reportNames, ticket.ID)
		ticketStates = append(ticketStates, map[string]any{
			"id":     ticket.ID,
			"title":  ticket.Title,
			"status": ticket.Status,
		})
	}
	structuredState := string(mustJSONBytes(map[string]any{
		"done":     req.Backlog.DoneQueue,
		"blocked":  req.Backlog.BlockedQueue,
		"deferred": req.Backlog.DeferredQueue,
		"ready":    req.Backlog.ReadyQueue,
		"tickets":  ticketStates,
	}))
	autoCommitLines := []string{
		fmt.Sprintf("- Auto-commit setting: %t", req.AutoCommit.Enabled),
	}
	if req.AutoCommit.Enabled {
		switch {
		case req.AutoCommit.Performed:
			autoCommitLines = append(autoCommitLines, fmt.Sprintf("- Engine committed all session changes at %s.", req.AutoCommit.CommitSHA))
		case strings.TrimSpace(req.AutoCommit.Error) != "":
			autoCommitLines = append(autoCommitLines, fmt.Sprintf("- Auto-commit was attempted but failed: %s", req.AutoCommit.Error))
		default:
			autoCommitLines = append(autoCommitLines, "- Auto-commit was enabled but there was nothing to commit (clean working tree).")
		}
	} else {
		autoCommitLines = append(autoCommitLines, "- The user has auto-commit OFF. The engine deliberately did not commit; changes remain in the working tree for manual review.")
	}
	prompt := strings.Join([]string{
		"Return strict JSON only with this shape:",
		`{"summary":"...","report_body":"# Final Report\n..."}`,
		"",
		fmt.Sprintf("Session ID: %s", req.Session.SessionID),
		fmt.Sprintf("Original request: %s", req.Session.UserRequest),
		"",
		"Agent Memory:",
		emptyIfBlank(req.AgentMemory),
		"",
		"Rules:",
		"- Summarize what was completed, what was deferred or blocked, and the remaining risks.",
		"- Reference validation honestly from real test outcomes when present.",
		"- Keep the report readable markdown.",
		"- Reflect the auto-commit outcome below truthfully in the report (whether changes were committed by the engine or left in the working tree).",
		"- Use Structured backlog state as authoritative for ticket status. Do not infer status from truncated prose in the review log.",
		"",
		"Auto-commit outcome:",
		strings.Join(autoCommitLines, "\n"),
		"",
		"Structured backlog state:",
		structuredState,
		"",
		"Review log:",
		reviewLog,
		"",
		"Ticket ids:",
		strings.Join(reportNames, ", "),
	}, "\n")
	return prompt, map[string]any{
		"session_id":  req.Session.SessionID,
		"tickets":     len(req.Tickets),
		"auto_commit": req.AutoCommit.Enabled,
	}
}

func (r *DeepSeekRunner) repoContext(repoPath string) (string, string) {
	if r.git == nil {
		return "(git unavailable)", "(manifest unavailable)"
	}
	snapshot, err := r.git.InspectRepo(repoPath)
	if err != nil {
		return err.Error(), "(manifest unavailable)"
	}
	manifest := []string{}
	if files, err := r.git.ListFiles(repoPath); err == nil {
		sort.Strings(files)
		manifest = limitStrings(files, 200)
	}
	status := "(clean)"
	if len(snapshot.StatusLines) > 0 {
		status = strings.Join(snapshot.StatusLines, "\n")
	}
	headLine := fmt.Sprintf("Head: %s", snapshot.HeadCommit)
	if snapshot.HeadCommit == "" {
		headLine = "Head: NONE — greenfield repo with zero commits. Do NOT run git log, git diff, git show, or git branch — they will error. Use list/glob/file_read to inspect files instead."
	}
	return strings.Join([]string{
		fmt.Sprintf("Root: %s", snapshot.Root),
		fmt.Sprintf("Branch: %s", snapshot.HeadBranch),
		fmt.Sprintf("Dirty: %t", snapshot.Dirty),
		headLine,
		"Status:",
		status,
	}, "\n"), strings.Join(manifest, "\n")
}

func (r *DeepSeekRunner) writeRawResponse(workspacePath, name string, payload any) {
	dir := filepath.Join(workspacePath, "responses")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, name), mustJSONBytes(payload), 0o600)
}

type questionGateResponse struct {
	Needed          bool                  `json:"needed"`
	Summary         string                `json:"summary"`
	Questions       []engine.QuestionItem `json:"questions"`
	LockedDecisions []string              `json:"locked_decisions"`
	Assumptions     []string              `json:"assumptions"`
}

type directiveResponse struct {
	Summary                 string   `json:"summary"`
	SessionIdentity         []string `json:"session_identity"`
	UserRequest             []string `json:"user_request"`
	LockedDecisions         []string `json:"locked_decisions"`
	Assumptions             []string `json:"assumptions"`
	ProductFraming          []string `json:"product_framing"`
	Competitive             []string `json:"competitive"`
	ScopeRequired           []string `json:"scope_required"`
	ScopeBest               []string `json:"scope_best"`
	ScopeStretch            []string `json:"scope_stretch"`
	ArchitectureDirection   []string `json:"architecture_direction"`
	FullFeatureInventory    []string `json:"full_feature_inventory"`
	RisksAndUnknowns        []string `json:"risks_and_unknowns"`
	BacklogStrategy         []string `json:"backlog_strategy"`
	DispatchNotesForFinalCM []string `json:"dispatch_notes_for_final_cm"`
}

type backlogResponse struct {
	Summary string                 `json:"summary"`
	Tickets []backlogBlueprintWire `json:"tickets"`
}

type backlogBlueprintWire struct {
	Ref                 string            `json:"ref"`
	Title               string            `json:"title"`
	Type                string            `json:"type"`
	Area                string            `json:"area"`
	ScopeClass          string            `json:"scope_class"`
	Priority            string            `json:"priority"`
	Summary             string            `json:"summary"`
	AcceptanceCriteria  []string          `json:"acceptance_criteria"`
	Constraints         []string          `json:"constraints"`
	SuggestedProfile    string            `json:"suggested_profile"`
	EstimatedComplexity int               `json:"estimated_complexity"`
	RiskLevel           string            `json:"risk_level"`
	HandoffNotes        string            `json:"handoff_notes"`
	DependsOnRefs       []string          `json:"depends_on_refs"`
	SourceRefs          engine.SourceRefs `json:"source_refs"`
}

type reviewResponse struct {
	Summary       string                 `json:"summary"`
	Decision      string                 `json:"decision"`
	Reason        string                 `json:"reason"`
	FollowupNotes []string               `json:"followup_notes"`
	ChildTickets  []backlogBlueprintWire `json:"child_tickets"`
}

type finalizerResponse struct {
	Summary    string `json:"summary"`
	ReportBody string `json:"report_body"`
}

func parseQuestionGateResponse(content string) questionGateResponse {
	content = stripCodeFences(strings.TrimSpace(content))
	var parsed questionGateResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return questionGateResponse{
			Needed:  false,
			Summary: "Question-gate response did not parse as strict JSON; proceeding without user questions.",
			Assumptions: []string{
				"Question gate provider output was invalid JSON.",
			},
		}
	}
	return parsed
}

func parseDirectiveResponse(content string) directiveResponse {
	content = stripCodeFences(strings.TrimSpace(content))
	var parsed directiveResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return directiveResponse{
			Summary: "Directive provider output did not parse as strict JSON; falling back to minimal directive sections.",
			SessionIdentity: []string{
				"Provider output was invalid JSON.",
			},
			UserRequest: []string{
				"Original request preserved.",
			},
			LockedDecisions: []string{
				"Proceed using minimal fallback directive content.",
			},
			Assumptions: []string{
				"Provider output was invalid JSON.",
			},
			ProductFraming: []string{
				"Keep the orchestration flow serialized.",
			},
			ScopeRequired: []string{
				"Recover directive generation path.",
			},
			ArchitectureDirection: []string{
				"Keep session artifacts deterministic.",
			},
			BacklogStrategy: []string{
				"Generate the smallest safe backlog from fallback content.",
			},
		}
	}
	return parsed
}

func parseBacklogResponse(content string) backlogResponse {
	content = stripCodeFences(strings.TrimSpace(content))
	var parsed backlogResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return backlogResponse{}
	}
	return parsed
}

func parseReviewResponse(content string) reviewResponse {
	content = stripCodeFences(strings.TrimSpace(content))
	var parsed reviewResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return reviewResponse{
			Decision: string(engine.DecisionRevise),
			Reason:   "Review output did not parse as strict JSON.",
		}
	}
	return parsed
}

func parseFinalizerResponse(content string) finalizerResponse {
	content = stripCodeFences(strings.TrimSpace(content))
	var parsed finalizerResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return finalizerResponse{
			Summary:    "Finalizer output did not parse as strict JSON.",
			ReportBody: "# Final Report\n\nFinalizer output did not parse as strict JSON.",
		}
	}
	return parsed
}

func extractContent(resp *chatprovider.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	switch content := resp.Choices[0].Message.Content.(type) {
	case string:
		return strings.TrimSpace(content)
	default:
		return ""
	}
}

// stripCodeFences pulls a JSON payload out of model output. The CM now
// explores with tools before answering, so the final message often arrives
// wrapped in prose like "After inspecting the repo, here is the result:
// ```json {...} ```". This first peels off any markdown fence, and if the
// remainder still isn't pure JSON, scans for the first balanced {...} object
// (respecting strings and escapes) and returns that.
func stripCodeFences(content string) string {
	content = strings.TrimSpace(content)
	if idx := strings.Index(content, "```"); idx >= 0 {
		content = content[idx+3:]
		content = strings.TrimPrefix(content, "json")
		content = strings.TrimPrefix(content, "JSON")
		if end := strings.LastIndex(content, "```"); end >= 0 {
			content = content[:end]
		}
		content = strings.TrimSpace(content)
	}
	if strings.HasPrefix(content, "{") && strings.HasSuffix(content, "}") {
		return content
	}
	if obj := extractFirstJSONObject(content); obj != "" {
		return obj
	}
	return content
}

// extractFirstJSONObject returns the first top-level balanced {...} substring
// in s, or "" if none is found. It tracks string literals and escape
// sequences so braces inside strings do not confuse the depth counter.
func extractFirstJSONObject(s string) string {
	start := -1
	depth := 0
	inString := false
	escape := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					return s[start : i+1]
				}
			}
		}
	}
	return ""
}

func mustJSONBytes(v any) []byte {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return data
}

func normalizeQuestions(items []engine.QuestionItem, max int) []engine.QuestionItem {
	out := make([]engine.QuestionItem, 0, len(items))
	for i, item := range items {
		if len(out) >= max {
			break
		}
		prompt := strings.TrimSpace(item.Prompt)
		if prompt == "" {
			continue
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("Q%d", i+1)
		}
		out = append(out, engine.QuestionItem{
			ID:        id,
			Prompt:    prompt,
			Rationale: strings.TrimSpace(item.Rationale),
		})
	}
	return out
}

func normalizeLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			out = append(out, line)
			continue
		}
		out = append(out, "- "+line)
	}
	return out
}

func normalizeBacklogBlueprints(items []backlogBlueprintWire) []engine.TicketBlueprint {
	out := make([]engine.TicketBlueprint, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Title) == "" {
			continue
		}
		out = append(out, engine.TicketBlueprint{
			Ref:                 strings.TrimSpace(item.Ref),
			Title:               strings.TrimSpace(item.Title),
			Type:                strings.TrimSpace(item.Type),
			Area:                strings.TrimSpace(item.Area),
			ScopeClass:          strings.TrimSpace(item.ScopeClass),
			Priority:            strings.TrimSpace(item.Priority),
			Summary:             strings.TrimSpace(item.Summary),
			AcceptanceCriteria:  normalizeLines(item.AcceptanceCriteria),
			Constraints:         normalizeLines(item.Constraints),
			SuggestedProfile:    strings.TrimSpace(item.SuggestedProfile),
			EstimatedComplexity: item.EstimatedComplexity,
			RiskLevel:           strings.TrimSpace(item.RiskLevel),
			HandoffNotes:        strings.TrimSpace(item.HandoffNotes),
			DependsOnRefs:       item.DependsOnRefs,
			SourceRefs:          item.SourceRefs,
		})
	}
	return out
}

func directiveStageInstruction(req engine.CMDirectiveRequest) string {
	switch req.Phase {
	case "initial":
		return "Create the first strong directive pass from the user request, repo context, and any question answers."
	case "review":
		return "Critique the existing directive, tighten weak assumptions, add missing required capabilities, and improve dispatch clarity."
	case "final":
		return "Freeze the directive for execution, make the backlog strategy concrete, and write strong dispatch notes for serialized implementer work."
	default:
		return "Improve the directive rigorously."
	}
}

func formatQuestionBatch(batch engine.QuestionBatch) string {
	lines := []string{
		fmt.Sprintf("Asked by: %s", batch.AskedBy),
		fmt.Sprintf("Summary: %s", batch.Summary),
	}
	if len(batch.Questions) == 0 {
		lines = append(lines, "Questions: none")
	} else {
		lines = append(lines, "Questions:")
		for _, q := range batch.Questions {
			lines = append(lines, fmt.Sprintf("- [%s] %s", q.ID, q.Prompt))
		}
	}
	if len(batch.Answers) == 0 {
		lines = append(lines, "Answers: none")
	} else {
		lines = append(lines, "Answers:")
		for _, answer := range batch.Answers {
			lines = append(lines, fmt.Sprintf("- [%s] %s", answer.QuestionID, answer.Answer))
		}
	}
	return strings.Join(lines, "\n")
}

func limitStrings(items []string, max int) []string {
	if max <= 0 || len(items) <= max {
		return items
	}
	return append([]string{}, items[:max]...)
}

func buildChangedFileContexts(repoPath string, files []string) []string {
	selected := limitStrings(files, 4)
	contexts := make([]string, 0, len(selected))
	maxBytes := 12000
	if len(selected) == 1 {
		maxBytes = 30000
	}
	for _, rel := range selected {
		path := filepath.Join(repoPath, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(data)
		if len(text) > maxBytes {
			text = text[:maxBytes]
		}
		contexts = append(contexts, fmt.Sprintf("FILE: %s\n%s", rel, text))
	}
	return contexts
}

// ── CM exploration tool loop ──────────────────────────────────────────────
//
// Browse-only tools (list, glob, grep, file_read) the CM can call to inspect
// the user repo before producing strict-JSON output. The model is expected to
// gather context, then return a final assistant message containing only the
// JSON payload.

const cmExploreRounds = 8

func cmExploreToolDefinitions() []chatprovider.ToolDefinition {
	return []chatprovider.ToolDefinition{
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "list",
				Description: "List repo files as a compact tree overview, capped to a small result set. Use this for initial discovery.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Optional repo-relative directory path. Use . for repo root.",
						},
						"ignore": map[string]any{
							"type":        "array",
							"description": "Optional extra ignore glob patterns.",
							"items":       map[string]any{"type": "string"},
						},
					},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "glob",
				Description: "Find files by name or path pattern, capped to a small result set.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{"type": "string", "description": "Glob pattern such as **/*.html or src/**/*.js."},
						"path":    map[string]any{"type": "string", "description": "Optional repo-relative search root. Use . for repo root."},
					},
					"required":             []string{"pattern"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "grep",
				Description: "Search file contents by regex, capped to a small result set.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{"type": "string", "description": "Regular expression pattern to search for."},
						"path":    map[string]any{"type": "string", "description": "Optional repo-relative search root. Use . for repo root."},
						"include": map[string]any{"type": "string", "description": "Optional glob include pattern such as *.ts or *.{ts,tsx}."},
					},
					"required":             []string{"pattern"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "guidance_list",
				Description: "List available brief guidance categories. Use this when the task domain may match project guidance.",
				Parameters: map[string]any{
					"type":                 "object",
					"properties":           map[string]any{},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "guidance_get",
				Description: "Read one or more guidance categories by id. Use only for categories relevant to the current task.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ids": map[string]any{
							"type":        "array",
							"description": "Guidance category ids to read.",
							"items":       map[string]any{"type": "string"},
						},
					},
					"required":             []string{"ids"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "file_read",
				Description: "Read a file or directory listing with numbered line output. Use this after list/glob/grep to inspect concrete contents.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"filePath": map[string]any{"type": "string", "description": "Repo-relative path to a file or directory."},
						"offset":   map[string]any{"type": "integer", "description": "1-indexed starting line or entry offset."},
						"limit":    map[string]any{"type": "integer", "description": "Maximum number of lines or entries to read."},
					},
					"required":             []string{"filePath", "offset", "limit"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "shell",
				Description: "Run a shell command using the platform native shell (cmd /C on Windows, sh -c on Linux/macOS). Use for read-oriented commands such as git log, git diff, git status, git show, npm ls, or to read files outside the repo. Do NOT modify the repo, do NOT commit, push, or stage — CM agents are read-only. Stdin is ignored; the command must be non-interactive. Use get_system_info to learn the OS and shell before running platform-specific commands. git log and git diff will error on repos with zero commits — skip them when the repo is empty.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command":    map[string]any{"type": "string", "description": "The shell command to run."},
						"cwd":        map[string]any{"type": "string", "description": "Optional working directory (absolute or repo-relative). Defaults to repo root."},
						"timeout_ms": map[string]any{"type": "integer", "description": "Timeout in milliseconds. Default 30000, max 120000."},
					},
					"required":             []string{"command"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "get_system_info",
				Description: "Returns the current OS, shell, date, time, and timezone. Call this before running shell commands that may differ across platforms.",
				Parameters: map[string]any{
					"type":                 "object",
					"properties":           map[string]any{},
					"additionalProperties": false,
				},
			},
		},
	}
}

func executeCMExploreTool(repoPath string, call chatprovider.ToolCall) (string, engine.ToolCall) {
	name := strings.TrimSpace(call.Function.Name)
	switch name {
	case "list":
		var args repotools.ListArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid list arguments"}), engine.ToolCall{Name: "list", Status: "failed"}
		}
		out, record, err := repotools.ExecuteList(repoPath, args)
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "result": out}), record
	case "glob":
		var args repotools.GlobArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid glob arguments"}), engine.ToolCall{Name: "glob", Status: "failed"}
		}
		out, record, err := repotools.ExecuteGlob(repoPath, args)
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "result": out}), record
	case "grep":
		var args repotools.GrepArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid grep arguments"}), engine.ToolCall{Name: "grep", Status: "failed"}
		}
		out, record, err := repotools.ExecuteGrep(repoPath, args)
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "result": out}), record
	case "file_read":
		var args repotools.FileReadArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid file_read arguments"}), engine.ToolCall{Name: "file_read", Status: "failed"}
		}
		out, record, err := repotools.ExecuteFileRead(repoPath, args)
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "result": out}), record
	case "guidance_list":
		out, record, err := guidance.ExecuteList()
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "categories": out}), record
	case "guidance_get":
		var args guidance.GetArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid guidance_get arguments"}), engine.ToolCall{Name: "guidance_get", Status: "failed"}
		}
		out, record, err := guidance.ExecuteGet(args)
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "documents": out}), record
	case "shell":
		var args shellexec.ShellArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid shell arguments"}), engine.ToolCall{Name: "shell", Status: "failed"}
		}
		output, record, err := shellexec.ExecuteShell(repoPath, args)
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "output": output}), record
	case "get_system_info":
		return repotools.MarshalToolResult(shellexec.SystemInfo()), engine.ToolCall{Name: "get_system_info", Status: "done"}
	default:
		msg := fmt.Sprintf("Tool %q does not exist for CM exploration. Only list, glob, grep, file_read, guidance_list, guidance_get, shell, get_system_info are available. Stop calling tools and produce the strict JSON output now.", name)
		return repotools.MarshalToolResult(map[string]any{"ok": false, "error": msg, "skipped": true}), engine.ToolCall{Name: name, Status: "skipped"}
	}
}

// chatWithExploration runs an exploration loop where the model is given the
// browse-only tools and asked to produce a strict-JSON final message. Returns
// the final assistant content (the JSON payload) plus the raw rounds for
// artifact persistence and the tool-call records for telemetry.
func (r *DeepSeekRunner) chatWithExploration(ctx context.Context, repoPath, systemPrompt, userPrompt, agentMemory, agentID, phase, ticketID string, remember engine.MemoryFunc, activity engine.ActivityFunc, gate func(context.Context) error) (string, []map[string]any, []engine.ToolCall, *chatprovider.ChatResponse, error) {
	tools := cmExploreToolDefinitions()
	if strings.TrimSpace(agentMemory) != "" {
		userPrompt = strings.Join([]string{
			"Agent Memory:",
			agentMemory,
			"",
			userPrompt,
		}, "\n")
	}
	messages := []chatprovider.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	rounds := []map[string]any{}
	toolCalls := []engine.ToolCall{}
	var lastResp *chatprovider.ChatResponse
	finalContent := ""

	for round := 1; round <= cmExploreRounds; round++ {
		emitCMActivity(activity, engine.Activity{
			Kind:   "thinking",
			Status: "running",
			Detail: fmt.Sprintf("Thinking · round %d", round),
		})
		resp, err := r.client.ChatCompletion(ctx, chatprovider.ChatRequest{
			Model:       r.model,
			Messages:    messages,
			Tools:       tools,
			ToolChoice:  "auto",
			Thinking:    &chatprovider.ThinkingConfig{Type: "disabled"},
			Stream:      false,
			MaxTokens:   r.maxTokens,
			Temperature: r.temperature,
		})
		if err != nil {
			return "", rounds, toolCalls, lastResp, err
		}
		lastResp = resp
		content := extractContent(resp)
		responseToolCalls := extractResponseToolCalls(resp)
		rounds = append(rounds, map[string]any{
			"round":       round,
			"response_id": resp.ID,
			"raw_content": content,
			"tool_calls":  responseToolCalls,
			"usage":       resp.Usage,
		})
		messages = append(messages, chatprovider.Message{
			Role:      "assistant",
			Content:   content,
			ToolCalls: responseToolCalls,
		})
		if len(responseToolCalls) == 0 {
			finalContent = content
			break
		}
		for _, call := range responseToolCalls {
			toolName, target, detail := describeCMExploreCall(call)
			emitCMActivity(activity, engine.Activity{
				Kind:   "tool_start",
				Tool:   toolName,
				Target: target,
				Detail: detail,
				Status: "running",
			})
			result, record := executeCMExploreTool(repoPath, call)
			toolCalls = append(toolCalls, record)
			rememberToolCall(remember, agentID, phase, ticketID, record, result)
			emitCMActivity(activity, engine.Activity{
				Kind:   "tool_done",
				Tool:   toolName,
				Target: target,
				Detail: detail,
				Status: "done",
			})
			messages = append(messages, chatprovider.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result,
			})
		}
		if gate != nil {
			if err := gate(ctx); err != nil {
				return "", rounds, toolCalls, lastResp, err
			}
		}
	}

	if finalContent == "" {
		// Force one final non-tool response if we exhausted rounds while still
		// asking for tools — the model has enough context now.
		messages = append(messages, chatprovider.Message{
			Role:    "user",
			Content: "Stop calling tools. Produce the final answer now using everything you have explored.",
		})
		emitCMActivity(activity, engine.Activity{
			Kind:   "thinking",
			Status: "running",
			Detail: "Synthesizing final answer",
		})
		resp, err := r.client.ChatCompletion(ctx, chatprovider.ChatRequest{
			Model:       r.model,
			Messages:    messages,
			Thinking:    &chatprovider.ThinkingConfig{Type: "disabled"},
			Stream:      false,
			MaxTokens:   r.maxTokens,
			Temperature: r.temperature,
		})
		if err != nil {
			return "", rounds, toolCalls, lastResp, err
		}
		lastResp = resp
		finalContent = extractContent(resp)
		rounds = append(rounds, map[string]any{
			"round":       cmExploreRounds + 1,
			"response_id": resp.ID,
			"raw_content": finalContent,
			"forced":      true,
			"usage":       resp.Usage,
		})
	}

	return finalContent, rounds, toolCalls, lastResp, nil
}

func emitCMActivity(fn engine.ActivityFunc, a engine.Activity) {
	if fn == nil {
		return
	}
	fn(a)
}

func emptyIfBlank(v string) string {
	if strings.TrimSpace(v) == "" {
		return "(none)"
	}
	return v
}

func rememberToolCall(remember engine.MemoryFunc, agentID, phase, ticketID string, record engine.ToolCall, result string) {
	if remember == nil {
		return
	}
	content := strings.TrimSpace(record.Name)
	if strings.TrimSpace(record.Target) != "" {
		content += " " + strings.TrimSpace(record.Target)
	}
	if strings.TrimSpace(record.Status) != "" {
		content += " [" + strings.TrimSpace(record.Status) + "]"
	}
	switch record.Name {
	case "guidance_get", "guidance_list":
		if strings.TrimSpace(result) != "" {
			content += "\n" + result
		}
	case "file_read":
		content += "\nHistorical file observation only; re-read before relying on current code."
	}
	kind := "tool_call"
	if record.Name == "guidance_get" || record.Name == "guidance_list" {
		kind = "guidance"
	}
	remember(engine.MemoryEvent{
		AgentID:  agentID,
		Kind:     kind,
		Phase:    phase,
		TicketID: ticketID,
		Content:  content,
		Meta: map[string]any{
			"tool":   record.Name,
			"target": record.Target,
			"status": record.Status,
		},
	})
}

func describeCMExploreCall(call chatprovider.ToolCall) (string, string, string) {
	name := strings.TrimSpace(call.Function.Name)
	args := strings.TrimSpace(call.Function.Arguments)
	if args == "" {
		return name, "", cmActionVerb(name)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return name, "", cmActionVerb(name)
	}
	switch name {
	case "list":
		target := cmStringVal(raw, "path")
		if target == "" {
			target = "."
		}
		return name, target, fmt.Sprintf("Listing %s", target)
	case "glob":
		target := cmStringVal(raw, "pattern")
		return name, target, fmt.Sprintf("Globbing %s", target)
	case "grep":
		target := cmStringVal(raw, "pattern")
		return name, target, fmt.Sprintf("Searching for %q", target)
	case "file_read":
		target := cmStringVal(raw, "filePath")
		offset := cmIntVal(raw, "offset")
		limit := cmIntVal(raw, "limit")
		detail := fmt.Sprintf("Reading %s", target)
		if limit > 0 {
			start := offset
			if start < 1 {
				start = 1
			}
			detail = fmt.Sprintf("Reading %s · L%d-%d", target, start, start+limit-1)
		}
		return name, target, detail
	case "guidance_list":
		return name, "guidance", "Listing guidance"
	case "guidance_get":
		target := strings.Join(cmStringSliceVal(raw, "ids"), ",")
		if target == "" {
			target = "guidance"
		}
		return name, target, fmt.Sprintf("Reading guidance: %s", target)
	case "shell":
		target := cmStringVal(raw, "command")
		return name, target, fmt.Sprintf("Running: %s", target)
	default:
		return name, "", cmActionVerb(name)
	}
}

func cmActionVerb(tool string) string {
	switch tool {
	case "list":
		return "Listing repo"
	case "glob":
		return "Globbing"
	case "grep":
		return "Searching"
	case "file_read":
		return "Reading"
	case "guidance_list":
		return "Listing guidance"
	case "guidance_get":
		return "Reading guidance"
	case "shell":
		return "Running command"
	}
	if tool == "" {
		return "Working"
	}
	return tool
}

func cmStringVal(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func cmStringSliceVal(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func cmIntVal(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

func summarizePlanAnswer(answer string) string {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "Plan-mode answer was empty."
	}
	return limitPromptLine(answer, 600)
}

func limitPromptLine(s string, maxLen int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if maxLen > 0 && len(s) > maxLen {
		return strings.TrimSpace(s[:maxLen]) + "..."
	}
	return s
}

func extractResponseToolCalls(resp *chatprovider.ChatResponse) []chatprovider.ToolCall {
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}
	return resp.Choices[0].Message.ToolCalls
}
