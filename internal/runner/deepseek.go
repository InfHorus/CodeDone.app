package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

func (r *DeepSeekRunner) RunImplementer(ctx context.Context, req engine.ImplementerRunRequest) (engine.ImplementerRunResult, error) {
	prompt, promptMeta := r.buildPrompt(req)
	tools := implementerToolDefinitions()
	messages := []chatprovider.Message{
		{
			Role:    "system",
			Content: "You are the Implementer agent for CodeDone. You execute exactly one ticket, stay within scope, and use tool calls to inspect, modify, or complete the ticket. Before editing, verify whether the requested behavior already exists. If it already exists, call complete with evidence instead of making cosmetic churn. If it partly exists, preserve working code and edit only the missing or incorrect parts. For existing files, use file_edit only; never call file_write for a path that already exists or appears in the manifest. Use file_write only for brand new files. Do not describe code changes in prose instead of calling tools. You may use the shell tool to run read-oriented or build commands such as: git diff, git log, git status, go build ./..., go test ./..., npm run build, npm test, tsc --noEmit, cargo build, make, or any command that helps you understand the current state or validate your changes. You are NEVER responsible for git history: do not run git commit, git push, git add, git reset, git checkout, or git branch via shell — the engine owns all git operations. Stop emitting tool calls once your file changes for the ticket are applied or once complete has been called.",
		},
		{
			Role:    "user",
			Content: prompt,
		},
	}
	toolCalls := []engine.ToolCall{}
	roundPayloads := []map[string]any{}
	appliedFiles := []string{}
	filePreviews := []engine.FileDiffPreview{}
	var finalContent string
	var completion *completeArgs

	for round := 1; round <= 12; round++ {
		emitActivity(req.Activity, engine.Activity{
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
			return engine.ImplementerRunResult{}, err
		}
		content := extractContent(resp)
		responseToolCalls := extractResponseToolCalls(resp)
		roundPayloads = append(roundPayloads, map[string]any{
			"round":       round,
			"response_id": resp.ID,
			"model":       resp.Model,
			"raw_content": content,
			"tool_calls":  responseToolCalls,
			"usage":       resp.Usage,
			"choices":     resp.Choices,
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
			toolName, target, detail := describeToolCall(call)
			emitActivity(req.Activity, engine.Activity{
				Kind:   "tool_start",
				Tool:   toolName,
				Target: target,
				Detail: detail,
				Status: "running",
			})
			resultMessage, record, changed, previews, err := r.executeToolCall(req, call)
			toolCalls = append(toolCalls, record)
			rememberToolCall(req.Remember, req.Implementer.ID, "implementing", req.Ticket.ID, record, resultMessage)
			doneStatus := "done"
			doneKind := "tool_done"
			if err != nil {
				doneStatus = "failed"
				doneKind = "tool_failed"
			}
			emitActivity(req.Activity, engine.Activity{
				Kind:   doneKind,
				Tool:   toolName,
				Target: target,
				Detail: detail,
				Status: doneStatus,
			})
			if len(previews) > 0 {
				filePreviews = append(filePreviews, previews...)
			}
			if err != nil {
				if isUnsupportedToolError(err) || isRecoverableToolError(err) {
					messages = append(messages, chatprovider.Message{
						Role:       "tool",
						ToolCallID: call.ID,
						Content:    resultMessage,
					})
					continue
				}
				return engine.ImplementerRunResult{
					Status:             engine.ReportFailed,
					Summary:            fmt.Sprintf("DeepSeek returned tool calls for %s, but executing %s failed: %v", req.Ticket.ID, call.Function.Name, err),
					FilesChanged:       dedupeStrings(appliedFiles),
					GitDiffRef:         "working-tree",
					ToolCalls:          toolCalls,
					FilePreviews:       filePreviews,
					ImportantDecisions: []string{"Provider response was received, but one tool call failed during execution."},
					KnownRisks:         []string{"The ticket did not complete because tool execution failed."},
					RemainingWork:      []string{"Retry the ticket with a corrected tool call."},
					SuggestedNextStep:  "Inspect the saved raw DeepSeek response artifact and retry with corrected tool usage.",
				}, nil
			}
			if done, args := parseCompleteToolCall(call); done {
				completion = &args
			}
			appliedFiles = append(appliedFiles, changed...)
			messages = append(messages, chatprovider.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    resultMessage,
			})
		}
	}

	toolPayloads := flattenToolCalls(roundPayloads)
	_ = os.MkdirAll(filepath.Join(req.WorkspacePath, "responses"), 0o755)
	rawPath := filepath.Join(req.WorkspacePath, "responses", fmt.Sprintf("%s.attempt-%d.deepseek.json", req.Ticket.ID, req.Attempt))
	_ = os.WriteFile(rawPath, mustJSONBytes(map[string]any{
		"ticket_id":   req.Ticket.ID,
		"attempt":     req.Attempt,
		"model":       r.model,
		"prompt_meta": promptMeta,
		"rounds":      roundPayloads,
		"tool_calls":  toolPayloads,
	}), 0o600)

	if len(toolCalls) == 0 {
		return engine.ImplementerRunResult{
			Status:            engine.ReportFailed,
			Summary:           fmt.Sprintf("DeepSeek returned no tool calls for %s. The implementer described work in prose but did not execute any file operation.", req.Ticket.ID),
			FilesChanged:      []string{},
			GitDiffRef:        "working-tree",
			ToolCalls:         []engine.ToolCall{},
			KnownRisks:        []string{"No file operations were executed."},
			RemainingWork:     []string{"Retry the ticket and require actual file tool calls."},
			SuggestedNextStep: "Inspect the saved DeepSeek response and adjust the implementer instructions if prose-only output continues.",
		}, nil
	}
	diffRef := "working-tree"
	diffStat := ""
	filesChanged := dedupeStrings(appliedFiles)

	if r.git != nil {
		if stat, err := r.git.DiffStat(req.RepoPath); err == nil {
			diffStat = stat
		}
	}

	if len(filesChanged) == 0 && completion != nil && strings.EqualFold(strings.TrimSpace(completion.Status), "done") {
		summary := strings.TrimSpace(completion.Summary)
		if summary == "" {
			summary = fmt.Sprintf("%s is already satisfied in the current repo state.", req.Ticket.ID)
		}
		decisions := []string{"Implementer inspected the repo and completed the ticket without file churn because the requested behavior was already present."}
		if len(completion.Evidence) > 0 {
			decisions = append(decisions, "Evidence: "+strings.Join(limitStrings(completion.Evidence, 8), " | "))
		}
		return engine.ImplementerRunResult{
			Status:             engine.ReportDone,
			Summary:            summary,
			FilesChanged:       []string{},
			GitDiffRef:         diffRef,
			GitDiffStat:        diffStat,
			ToolCalls:          toolCalls,
			FilePreviews:       filePreviews,
			ImportantDecisions: decisions,
			KnownRisks:         normalizeCompletionLines(completion.KnownRisks),
			RemainingWork:      normalizeCompletionLines(completion.RemainingWork),
			SuggestedNextStep:  "Run engine-owned validation and proceed to CM review.",
		}, nil
	}
	if len(filesChanged) == 0 && completion != nil && strings.EqualFold(strings.TrimSpace(completion.Status), "blocked") {
		summary := strings.TrimSpace(completion.Summary)
		if summary == "" {
			summary = fmt.Sprintf("%s is blocked by a hard prerequisite.", req.Ticket.ID)
		}
		return engine.ImplementerRunResult{
			Status:             engine.ReportBlocked,
			Summary:            summary,
			FilesChanged:       []string{},
			GitDiffRef:         diffRef,
			GitDiffStat:        diffStat,
			ToolCalls:          toolCalls,
			FilePreviews:       filePreviews,
			ImportantDecisions: normalizeCompletionLines(completion.Evidence),
			KnownRisks:         normalizeCompletionLines(completion.KnownRisks),
			RemainingWork:      normalizeCompletionLines(completion.RemainingWork),
			SuggestedNextStep:  "CM should decide whether to block, split, or revise the ticket.",
		}, nil
	}

	if len(filesChanged) == 0 {
		return engine.ImplementerRunResult{
			Status:             engine.ReportPartial,
			Summary:            fmt.Sprintf("DeepSeek used tool calls for %s but did not produce any file changes.", req.Ticket.ID),
			FilesChanged:       []string{},
			GitDiffRef:         diffRef,
			GitDiffStat:        diffStat,
			ToolCalls:          toolCalls,
			ImportantDecisions: []string{"Tool calls executed, but no file changes were produced."},
			KnownRisks:         []string{"The ticket cannot be accepted without verifiable file changes."},
			RemainingWork:      []string{"Retry with concrete file modifications."},
			SuggestedNextStep:  "Inspect the saved response rounds and retry with read-then-edit behavior.",
		}, nil
	}

	summary := strings.TrimSpace(finalContent)
	if summary == "" {
		summary = summarizeAppliedOperations(req.Ticket.ID, req.Ticket.Title, toolCalls, filesChanged)
	}
	return engine.ImplementerRunResult{
		Status:             engine.ReportDone,
		Summary:            summary,
		FilesChanged:       filesChanged,
		Commits:            []string{},
		GitDiffRef:         diffRef,
		GitDiffStat:        diffStat,
		TestsRun:           nil,
		ToolCalls:          toolCalls,
		FilePreviews:       filePreviews,
		ImportantDecisions: []string{"Changes were applied via DeepSeek tool calls rather than prose-only output."},
		KnownRisks:         []string{},
		RemainingWork:      []string{},
		SuggestedNextStep:  "Run engine-owned validation and proceed to CM review.",
	}, nil
}

func (r *DeepSeekRunner) buildPrompt(req engine.ImplementerRunRequest) (string, map[string]any) {
	directiveSummary := "Directive available in session workspace."
	directivePath := filepath.Join(req.WorkspacePath, "directive.md")
	if data, err := os.ReadFile(directivePath); err == nil {
		text := strings.TrimSpace(string(data))
		if len(text) > 3000 {
			text = text[:3000]
		}
		directiveSummary = text
	}

	var diffStat string
	var statusLines []string
	fileManifest := []string{}
	fileContexts := []string{}
	previousAttemptContext := ""
	previousWorkContext := r.loadPreviousAcceptedWorkContext(req.WorkspacePath, req.Ticket.ID)
	if r.git != nil {
		if stat, err := r.git.DiffStat(req.RepoPath); err == nil {
			diffStat = stat
		}
		if snapshot, err := r.git.InspectRepo(req.RepoPath); err == nil {
			statusLines = snapshot.StatusLines
		}
		if files, err := r.git.ListFiles(req.RepoPath); err == nil {
			fileManifest = limitStrings(files, 200)
			fileContexts = buildFileContexts(req.RepoPath, files, req.Ticket)
		}
	}
	if req.Attempt > 1 {
		previousAttemptContext = r.loadPreviousAttemptContext(req.WorkspacePath, req.Ticket.ID, req.Attempt-1)
	}

	prompt := strings.Join([]string{
		"Use the provided file tools to modify the repo. Do not return JSON plans or code in prose.",
		"",
		fmt.Sprintf("Session ID: %s", req.Session.SessionID),
		fmt.Sprintf("Ticket ID: %s", req.Ticket.ID),
		fmt.Sprintf("Attempt: %d", req.Attempt),
		fmt.Sprintf("Implementer: %s", req.Implementer.ID),
		fmt.Sprintf("Repo Path: %s", req.RepoPath),
		"",
		"Agent Memory:",
		emptyIfBlank(req.AgentMemory),
		"",
		"Ticket Summary:",
		req.Ticket.Summary,
		"",
		"Acceptance Criteria:",
		strings.Join(req.Ticket.AcceptanceCriteria, "\n"),
		"",
		"Constraints:",
		strings.Join(req.Ticket.Constraints, "\n"),
		"",
		"Directive Excerpt:",
		directiveSummary,
		"",
		"Current Git Diff Stat:",
		emptyIfBlank(diffStat),
		"",
		"Current Git Status:",
		emptyIfBlank(strings.Join(statusLines, "\n")),
		"",
		"Repo File Manifest:",
		emptyIfBlank(strings.Join(fileManifest, "\n")),
		"",
		"Selected File Contexts:",
		emptyIfBlank(strings.Join(fileContexts, "\n\n---\n\n")),
		"",
		"Previous Attempt Context:",
		emptyIfBlank(previousAttemptContext),
		"",
		"Previous Accepted Work Context:",
		emptyIfBlank(previousWorkContext),
		"",
		"Rules:",
		"- You must use one or more tool calls to perform the implementation.",
		"- If the repo is empty or you are unsure what files exist, start with list.",
		"- Use glob to find files by path pattern and grep to find relevant code by content before reading large files.",
		"- Use file_read only after you know which file or directory slice you need.",
		"- If the ticket clearly matches a guidance category, use guidance_list and then guidance_get for the relevant category before editing unless Agent Memory already contains the relevant guidance.",
		"- Treat guidance as a quality bar, not extra scope. If guidance conflicts with the ticket or existing repo patterns, follow the ticket and repo.",
		"- Stay strictly inside this ticket. Do not implement future tickets, adjacent features, or nice-to-have extras that are not required here.",
		"- First inspect whether this ticket is already fully or partially implemented.",
		"- If every acceptance criterion is already met, call complete with status=done and concise file-grounded evidence. Do not edit files just to satisfy the tool requirement.",
		"- If the ticket is partially implemented, preserve what works and only add or correct the missing behavior.",
		"- Before editing, do a brief sanity check of Previous Accepted Work Context against the files you inspect for this ticket.",
		"- Use that sanity check only to catch obvious conflicts, broken assumptions, or irrelevant prior edits that affect this ticket.",
		"- Do not broadly re-review prior work, and do not rewrite prior accepted work unless it directly blocks this ticket or must be adapted in the same files you are changing.",
		"- If you notice an obvious previous-work issue, mention it briefly in your final summary, Known Risks style, and explain whether your changes accounted for it.",
		"- Do not replace a working implementation with a similar rewrite unless the ticket explicitly requires that replacement.",
		"- Do not create a whole app or whole page when the ticket only asks for one slice of it.",
		"- Use repo-relative paths only.",
		"- For existing files, use file_edit with exact oldString and newString blocks taken from the provided file context.",
		"- Before any file_write, confirm the target path does not exist in Repo File Manifest and was not returned by list/glob/file_read.",
		"- oldString must match the current file content exactly.",
		"- Never emit file_edit with identical oldString and newString.",
		"- If oldString appears multiple times and you only intend one edit, include more surrounding context.",
		"- Use replaceAll=true only when every exact oldString match should be replaced.",
		"- Use file_write only for brand new files that do not already exist. If the file exists, file_write is wrong; use file_edit.",
		"- For file_write, provide the complete resulting file contents.",
		"- Do not rewrite an existing file with file_write to avoid file_edit precision requirements.",
		"- If a previous attempt failed with oldString not found or no changes, choose a different exact oldString block copied verbatim from the current file context.",
		"- If a previous attempt already added useful code, do not undo it or replace it with a parallel version. Read the current file and continue from the current state.",
		"- Do not describe finished code in prose without calling tools.",
		"- If you cannot safely edit from the available context, do not invent code that was not applied.",
		"- Do not emit Markdown fences.",
	}, "\n")

	return prompt, map[string]any{
		"ticket_id":     req.Ticket.ID,
		"attempt":       req.Attempt,
		"directive_len": len(directiveSummary),
		"manifest_len":  len(fileManifest),
		"context_files": len(fileContexts),
		"retry_context": previousAttemptContext != "",
		"previous_work": previousWorkContext != "",
	}
}

func (r *DeepSeekRunner) loadPreviousAcceptedWorkContext(workspacePath, currentTicketID string) string {
	backlogPath := filepath.Join(workspacePath, "backlog.json")
	data, err := os.ReadFile(backlogPath)
	if err != nil {
		return ""
	}
	var backlog engine.BacklogFile
	if err := json.Unmarshal(data, &backlog); err != nil {
		return ""
	}
	for i := len(backlog.DoneQueue) - 1; i >= 0; i-- {
		ticketID := strings.TrimSpace(backlog.DoneQueue[i])
		if ticketID == "" || ticketID == currentTicketID {
			continue
		}
		report := r.loadLatestReport(workspacePath, ticketID)
		if report == nil {
			continue
		}
		lines := []string{
			fmt.Sprintf("Previous accepted ticket: %s", ticketID),
			fmt.Sprintf("Implementer: %s", report.ImplementerID),
			fmt.Sprintf("Status: %s", report.Status),
			fmt.Sprintf("Summary: %s", limitPromptLine(strings.TrimSpace(report.Summary), 900)),
		}
		if len(report.FilesChanged) > 0 {
			lines = append(lines, "Files changed: "+strings.Join(limitStrings(report.FilesChanged, 12), ", "))
		}
		if strings.TrimSpace(report.GitDiffStat) != "" {
			lines = append(lines, "Diff stat: "+limitPromptLine(strings.TrimSpace(report.GitDiffStat), 600))
		}
		if len(report.KnownRisks) > 0 {
			lines = append(lines, "Known risks: "+strings.Join(limitStrings(report.KnownRisks, 5), " | "))
		}
		if len(report.RemainingWork) > 0 {
			lines = append(lines, "Remaining work noted: "+strings.Join(limitStrings(report.RemainingWork, 5), " | "))
		}
		return strings.Join(lines, "\n")
	}
	return ""
}

func (r *DeepSeekRunner) loadLatestReport(workspacePath, ticketID string) *engine.ImplementerReport {
	entries, err := os.ReadDir(filepath.Join(workspacePath, "reports"))
	if err != nil {
		return nil
	}
	prefix := ticketID + ".attempt-"
	bestAttempt := 0
	var bestReport *engine.ImplementerReport
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		attemptText := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".json")
		attempt, err := strconv.Atoi(attemptText)
		if err != nil || attempt < bestAttempt {
			continue
		}
		data, err := os.ReadFile(filepath.Join(workspacePath, "reports", name))
		if err != nil {
			continue
		}
		var report engine.ImplementerReport
		if err := json.Unmarshal(data, &report); err != nil {
			continue
		}
		bestAttempt = attempt
		bestReport = &report
	}
	return bestReport
}

func limitPromptLine(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return strings.TrimSpace(s[:maxLen-3]) + "..."
}

func (r *DeepSeekRunner) loadPreviousAttemptContext(workspacePath, ticketID string, attempt int) string {
	if attempt <= 0 {
		return ""
	}
	reportPath := filepath.Join(workspacePath, "reports", fmt.Sprintf("%s.attempt-%d.json", ticketID, attempt))
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return ""
	}
	var report engine.ImplementerReport
	if err := json.Unmarshal(data, &report); err != nil {
		return ""
	}
	lines := []string{
		fmt.Sprintf("Previous attempt: %d", attempt),
		fmt.Sprintf("Previous status: %s", report.Status),
		fmt.Sprintf("Previous summary: %s", strings.TrimSpace(report.Summary)),
	}
	if len(report.ToolCalls) > 0 {
		tools := make([]string, 0, len(report.ToolCalls))
		for _, call := range report.ToolCalls {
			item := call.Name
			if strings.TrimSpace(call.Target) != "" {
				item += "(" + call.Target + ")"
			}
			if strings.TrimSpace(call.Status) != "" {
				item += " [" + call.Status + "]"
			}
			tools = append(tools, item)
		}
		lines = append(lines, "Previous tool calls: "+strings.Join(tools, ", "))
	}
	if len(report.KnownRisks) > 0 {
		lines = append(lines, "Known risks: "+strings.Join(report.KnownRisks, " | "))
	}
	if strings.TrimSpace(report.SuggestedNextStep) != "" {
		lines = append(lines, "Suggested next step: "+strings.TrimSpace(report.SuggestedNextStep))
	}
	return strings.Join(lines, "\n")
}

func implementerToolDefinitions() []chatprovider.ToolDefinition {
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
							"items": map[string]any{
								"type": "string",
							},
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
						"pattern": map[string]any{
							"type":        "string",
							"description": "Glob pattern such as **/*.html or src/**/*.js.",
						},
						"path": map[string]any{
							"type":        "string",
							"description": "Optional repo-relative search root. Use . for repo root.",
						},
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
						"pattern": map[string]any{
							"type":        "string",
							"description": "Regular expression pattern to search for.",
						},
						"path": map[string]any{
							"type":        "string",
							"description": "Optional repo-relative search root. Use . for repo root.",
						},
						"include": map[string]any{
							"type":        "string",
							"description": "Optional glob include pattern such as *.ts or *.{ts,tsx}.",
						},
					},
					"required":             []string{"pattern"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "file_read",
				Description: "Read a file or directory listing with numbered line output. Use this when you need current code context before editing.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"filePath": map[string]any{
							"type":        "string",
							"description": "Repo-relative path to a file or directory.",
						},
						"offset": map[string]any{
							"type":        "integer",
							"description": "1-indexed starting line or entry offset.",
						},
						"limit": map[string]any{
							"type":        "integer",
							"description": "Maximum number of lines or entries to read.",
						},
					},
					"required":             []string{"filePath", "offset", "limit"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "guidance_list",
				Description: "List available brief guidance categories. Use this when the ticket domain may match project guidance.",
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
				Description: "Read one or more guidance categories by id. Use only for categories relevant to the current ticket.",
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
				Name:        "file_edit",
				Description: "Replace exact existing text in an existing file using oldString/newString semantics. Use this for every edit to an existing file.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"filePath": map[string]any{
							"type":        "string",
							"description": "Repo-relative file path.",
						},
						"oldString": map[string]any{
							"type":        "string",
							"description": "Exact existing text block copied verbatim from the current file.",
						},
						"newString": map[string]any{
							"type":        "string",
							"description": "Replacement text block.",
						},
						"replaceAll": map[string]any{
							"type":        "boolean",
							"description": "Set true only if every exact oldString match should be replaced.",
						},
					},
					"required":             []string{"filePath", "oldString", "newString", "replaceAll"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "file_write",
				Description: "Create a brand new file with complete contents. Never use for existing files; if a file exists, use file_edit.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"filePath": map[string]any{
							"type":        "string",
							"description": "Repo-relative file path for a new file.",
						},
						"content": map[string]any{
							"type":        "string",
							"description": "Complete file contents.",
						},
					},
					"required":             []string{"filePath", "content"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "complete",
				Description: "Finish the ticket without file changes when inspection proves the current repo already satisfies the acceptance criteria, or after all needed edits have been applied.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"status": map[string]any{
							"type":        "string",
							"description": "Use done only when the ticket is complete in the current repo state; use blocked only for a hard blocker.",
							"enum":        []string{"done", "blocked"},
						},
						"summary": map[string]any{
							"type":        "string",
							"description": "Concise completion summary.",
						},
						"evidence": map[string]any{
							"type":        "array",
							"description": "Short file-grounded evidence that acceptance criteria are met.",
							"items":       map[string]any{"type": "string"},
						},
						"knownRisks": map[string]any{
							"type":        "array",
							"description": "Known residual risks, if any.",
							"items":       map[string]any{"type": "string"},
						},
						"remainingWork": map[string]any{
							"type":        "array",
							"description": "Remaining work, empty when status is done.",
							"items":       map[string]any{"type": "string"},
						},
					},
					"required":             []string{"status", "summary", "evidence", "knownRisks", "remainingWork"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "file_delete",
				Description: "Delete an existing file by repo-relative path.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"filePath": map[string]any{
							"type":        "string",
							"description": "Repo-relative file path.",
						},
					},
					"required":             []string{"filePath"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: chatprovider.ToolFunctionSchema{
				Name:        "shell",
				Description: "Run a shell command using the platform native shell (cmd /C on Windows, sh -c on Linux/macOS). Use for build and validation commands (go build, go test, npm run build, npm test, tsc --noEmit, cargo build, make) and read-only git commands (git diff, git log, git status, git show). The cwd defaults to the repo root. Do NOT use for git commit, git push, git add, git reset, or git branch — the engine owns all git history operations. Stdin is ignored; the command must be non-interactive. Call get_system_info first if you need to know the OS or shell environment before running platform-specific commands.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{
							"type":        "string",
							"description": "The shell command to run.",
						},
						"cwd": map[string]any{
							"type":        "string",
							"description": "Optional working directory (absolute or repo-relative). Defaults to repo root.",
						},
						"timeout_ms": map[string]any{
							"type":        "integer",
							"description": "Timeout in milliseconds. Default 30000, max 120000. Increase for slow builds.",
						},
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

type fileEditArgs struct {
	FilePath   string `json:"filePath"`
	OldString  string `json:"oldString"`
	NewString  string `json:"newString"`
	ReplaceAll bool   `json:"replaceAll"`
}

type fileWriteArgs struct {
	FilePath string `json:"filePath"`
	Content  string `json:"content"`
}

type fileDeleteArgs struct {
	FilePath string `json:"filePath"`
}

type completeArgs struct {
	Status        string   `json:"status"`
	Summary       string   `json:"summary"`
	Evidence      []string `json:"evidence"`
	KnownRisks    []string `json:"knownRisks"`
	RemainingWork []string `json:"remainingWork"`
}

func parseToolCalls(resp *chatprovider.ChatResponse) ([]modelFileOperation, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty provider response")
	}
	toolCalls := resp.Choices[0].Message.ToolCalls
	ops := make([]modelFileOperation, 0, len(toolCalls))
	for _, call := range toolCalls {
		switch strings.TrimSpace(call.Function.Name) {
		case "file_edit":
			var args fileEditArgs
			if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
				return nil, fmt.Errorf("decode file_edit args: %w", err)
			}
			ops = append(ops, modelFileOperation{
				Type:       "file_edit",
				Path:       args.FilePath,
				OldString:  args.OldString,
				NewString:  args.NewString,
				ReplaceAll: args.ReplaceAll,
			})
		case "file_write":
			var args fileWriteArgs
			if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
				return nil, fmt.Errorf("decode file_write args: %w", err)
			}
			ops = append(ops, modelFileOperation{
				Type:    "file_write",
				Path:    args.FilePath,
				Content: args.Content,
			})
		case "file_delete":
			var args fileDeleteArgs
			if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
				return nil, fmt.Errorf("decode file_delete args: %w", err)
			}
			ops = append(ops, modelFileOperation{
				Type: "file_delete",
				Path: args.FilePath,
			})
		default:
			return nil, fmt.Errorf("unsupported tool call %q", call.Function.Name)
		}
	}
	return ops, nil
}

func summarizeAppliedOperations(ticketID, ticketTitle string, operations []engine.ToolCall, filesChanged []string) string {
	parts := make([]string, 0, len(operations))
	for _, op := range operations {
		parts = append(parts, fmt.Sprintf("%s(%s)", strings.TrimSpace(op.Name), strings.TrimSpace(op.Target)))
	}
	summary := fmt.Sprintf("Applied %d tool call(s) for %s.", len(operations), ticketID)
	if strings.TrimSpace(ticketTitle) != "" {
		summary = fmt.Sprintf("Applied %d tool call(s) for %s: %s.", len(operations), ticketID, ticketTitle)
	}
	if len(parts) > 0 {
		summary += " Operations: " + strings.Join(parts, ", ") + "."
	}
	if len(filesChanged) > 0 {
		summary += " Files changed: " + strings.Join(filesChanged, ", ") + "."
	}
	return summary
}

func extractResponseToolCalls(resp *chatprovider.ChatResponse) []chatprovider.ToolCall {
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}
	return resp.Choices[0].Message.ToolCalls
}

func flattenToolCalls(rounds []map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, round := range rounds {
		rawCalls, ok := round["tool_calls"].([]chatprovider.ToolCall)
		if !ok {
			continue
		}
		for _, call := range rawCalls {
			out = append(out, map[string]any{
				"id":   call.ID,
				"type": call.Type,
				"function": map[string]any{
					"name":      call.Function.Name,
					"arguments": call.Function.Arguments,
				},
			})
		}
	}
	return out
}

func (r *DeepSeekRunner) executeToolCall(req engine.ImplementerRunRequest, call chatprovider.ToolCall) (string, engine.ToolCall, []string, []engine.FileDiffPreview, error) {
	switch strings.TrimSpace(call.Function.Name) {
	case "list":
		var args repotools.ListArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid list arguments"}), engine.ToolCall{Name: "list", Status: "failed"}, nil, nil, fmt.Errorf("decode list args: %w", err)
		}
		output, record, err := repotools.ExecuteList(req.RepoPath, args)
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record, nil, nil, err
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "result": output}), record, nil, nil, nil
	case "glob":
		var args repotools.GlobArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid glob arguments"}), engine.ToolCall{Name: "glob", Status: "failed"}, nil, nil, fmt.Errorf("decode glob args: %w", err)
		}
		output, record, err := repotools.ExecuteGlob(req.RepoPath, args)
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record, nil, nil, err
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "result": output}), record, nil, nil, nil
	case "grep":
		var args repotools.GrepArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid grep arguments"}), engine.ToolCall{Name: "grep", Status: "failed"}, nil, nil, fmt.Errorf("decode grep args: %w", err)
		}
		output, record, err := repotools.ExecuteGrep(req.RepoPath, args)
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record, nil, nil, err
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "result": output}), record, nil, nil, nil
	case "file_read":
		var args repotools.FileReadArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid file_read arguments"}), engine.ToolCall{Name: "file_read", Status: "failed"}, nil, nil, fmt.Errorf("decode file_read args: %w", err)
		}
		output, record, err := repotools.ExecuteFileRead(req.RepoPath, args)
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record, nil, nil, err
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "result": output}), record, nil, nil, nil
	case "guidance_list":
		output, record, err := guidance.ExecuteList()
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record, nil, nil, fmt.Errorf("guidance_list: %w", err)
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "categories": output}), record, nil, nil, nil
	case "guidance_get":
		var args guidance.GetArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid guidance_get arguments"}), engine.ToolCall{Name: "guidance_get", Status: "failed"}, nil, nil, fmt.Errorf("decode guidance_get args: %w", err)
		}
		output, record, err := guidance.ExecuteGet(args)
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record, nil, nil, fmt.Errorf("guidance_get: %w", err)
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "documents": output}), record, nil, nil, nil
	case "complete":
		var args completeArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid complete arguments"}), engine.ToolCall{Name: "complete", Status: "failed"}, nil, nil, fmt.Errorf("decode complete args: %w", err)
		}
		status := strings.ToLower(strings.TrimSpace(args.Status))
		if status != "done" && status != "blocked" {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "complete status must be done or blocked"}), engine.ToolCall{Name: "complete", Status: "failed"}, nil, nil, errRecoverableTool{"complete status must be done or blocked"}
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "status": status}), engine.ToolCall{Name: "complete", Status: status}, nil, nil, nil
	case "shell":
		var args shellexec.ShellArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid shell arguments"}), engine.ToolCall{Name: "shell", Status: "failed"}, nil, nil, fmt.Errorf("decode shell args: %w", err)
		}
		output, record, err := shellexec.ExecuteShell(req.RepoPath, args)
		if err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record, nil, nil, err
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "output": output}), record, nil, nil, nil
	case "get_system_info":
		return repotools.MarshalToolResult(shellexec.SystemInfo()), engine.ToolCall{Name: "get_system_info", Status: "done"}, nil, nil, nil
	case "file_edit":
		var args fileEditArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid file_edit arguments"}), engine.ToolCall{Name: "file_edit", Status: "failed"}, nil, nil, fmt.Errorf("decode file_edit args: %w", err)
		}
		op := modelFileOperation{
			Type:       "file_edit",
			Path:       args.FilePath,
			OldString:  args.OldString,
			NewString:  args.NewString,
			ReplaceAll: args.ReplaceAll,
		}
		changed, previews, err := applyFileOperations(req.RepoPath, []modelFileOperation{op})
		record := summarizeToolCalls([]modelFileOperation{op}, "done")[0]
		if err != nil {
			record.Status = "failed"
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error(), "instruction": "Read the current file again if needed, then retry with an exact oldString from the current content. If the ticket is already satisfied, call complete instead of making a no-op edit."}), record, nil, previews, classifyToolError(err)
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "changed_files": changed}), record, changed, previews, nil
	case "file_write":
		var args fileWriteArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid file_write arguments"}), engine.ToolCall{Name: "file_write", Status: "failed"}, nil, nil, fmt.Errorf("decode file_write args: %w", err)
		}
		op := modelFileOperation{
			Type:    "file_write",
			Path:    args.FilePath,
			Content: args.Content,
		}
		changed, previews, err := applyFileOperations(req.RepoPath, []modelFileOperation{op})
		record := summarizeToolCalls([]modelFileOperation{op}, "done")[0]
		if err != nil {
			record.Status = "failed"
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error(), "instruction": "Do not use file_write for existing files. Use file_edit with exact oldString copied from the current file, or call complete if no edit is needed."}), record, nil, previews, classifyToolError(err)
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "changed_files": changed}), record, changed, previews, nil
	case "file_delete":
		var args fileDeleteArgs
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": "invalid file_delete arguments"}), engine.ToolCall{Name: "file_delete", Status: "failed"}, nil, nil, fmt.Errorf("decode file_delete args: %w", err)
		}
		op := modelFileOperation{
			Type: "file_delete",
			Path: args.FilePath,
		}
		changed, previews, err := applyFileOperations(req.RepoPath, []modelFileOperation{op})
		record := summarizeToolCalls([]modelFileOperation{op}, "done")[0]
		if err != nil {
			record.Status = "failed"
			return repotools.MarshalToolResult(map[string]any{"ok": false, "error": err.Error()}), record, nil, previews, err
		}
		return repotools.MarshalToolResult(map[string]any{"ok": true, "changed_files": changed}), record, changed, previews, nil
	default:
		name := strings.TrimSpace(call.Function.Name)
		record := engine.ToolCall{Name: name, Status: "skipped"}
		msg := fmt.Sprintf("Tool %q does not exist. Only list, glob, grep, file_read, guidance_list, guidance_get, file_edit, file_write, file_delete, and complete are available. Do not call git/bash/shell or any commit/push/stage tool — the engine owns git and decides at session end whether to commit. If your file changes for this ticket are already applied or the ticket is already satisfied, call complete.", name)
		return repotools.MarshalToolResult(map[string]any{"ok": false, "error": msg, "skipped": true}), record, nil, nil, errUnsupportedTool{name: name}
	}
}

type errUnsupportedTool struct{ name string }
type errRecoverableTool struct{ msg string }

func (e errUnsupportedTool) Error() string { return fmt.Sprintf("unsupported tool call %q", e.name) }
func (e errRecoverableTool) Error() string { return e.msg }

func isUnsupportedToolError(err error) bool {
	_, ok := err.(errUnsupportedTool)
	return ok
}

func isRecoverableToolError(err error) bool {
	_, ok := err.(errRecoverableTool)
	return ok
}

func classifyToolError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "file already exists") ||
		strings.Contains(msg, "oldString not found") ||
		strings.Contains(msg, "no changes") ||
		strings.Contains(msg, "multiple matches") ||
		strings.Contains(msg, "file_edit requires non-empty oldString") {
		return errRecoverableTool{msg: msg}
	}
	return err
}

func parseCompleteToolCall(call chatprovider.ToolCall) (bool, completeArgs) {
	if strings.TrimSpace(call.Function.Name) != "complete" {
		return false, completeArgs{}
	}
	var args completeArgs
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return true, completeArgs{}
	}
	return true, args
}

func normalizeCompletionLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseDeepSeekRunResult(content string) modelEditResponse {
	content = stripCodeFences(strings.TrimSpace(content))
	var parsed modelEditResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return modelEditResponse{
			Status:             string(engine.ReportPartial),
			Summary:            content,
			ImportantDecisions: []string{"Raw provider output could not be parsed as strict JSON."},
			KnownRisks:         []string{"Response parsing failed."},
			RemainingWork:      []string{"Normalize provider output schema."},
		}
	}
	return parsed
}

func stripCodeFences(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
	}
	return strings.TrimSpace(content)
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

func mustJSONBytes(v any) []byte {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return data
}

func emptyIfBlank(v string) string {
	if strings.TrimSpace(v) == "" {
		return "(empty)"
	}
	return v
}

func buildFileContexts(repoPath string, files []string, ticket engine.TicketFile) []string {
	selected := selectRelevantFiles(files, ticket)
	contexts := make([]string, 0, len(selected))
	for _, rel := range selected {
		path := filepath.Join(repoPath, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(data)
		if len(text) > 12000 {
			text = text[:12000]
		}
		contexts = append(contexts, fmt.Sprintf("FILE: %s\n%s", rel, text))
	}
	return contexts
}

func selectRelevantFiles(files []string, ticket engine.TicketFile) []string {
	type scored struct {
		path  string
		score int
	}
	keywords := ticketKeywords(ticket)
	scores := make([]scored, 0, len(files))
	for _, path := range files {
		lower := strings.ToLower(path)
		score := 0
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				score += 3
			}
		}
		if strings.HasSuffix(lower, ".go") {
			score++
		}
		if score > 0 {
			scores = append(scores, scored{path: path, score: score})
		}
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score == scores[j].score {
			return scores[i].path < scores[j].path
		}
		return scores[i].score > scores[j].score
	})

	selected := []string{}
	for _, item := range scores {
		selected = append(selected, item.path)
		if len(selected) >= 6 {
			break
		}
	}
	if len(selected) == 0 {
		for _, path := range files {
			if path == "main.go" || path == "go.mod" || strings.HasPrefix(path, "internal/") {
				selected = append(selected, path)
			}
			if len(selected) >= 6 {
				break
			}
		}
	}
	return dedupeStrings(selected)
}

func ticketKeywords(ticket engine.TicketFile) []string {
	raw := strings.ToLower(ticket.Title + " " + ticket.Summary + " " + ticket.Area + " " + ticket.SuggestedProfile)
	replacer := strings.NewReplacer("-", " ", "_", " ", "/", " ", ".", " ", ":", " ")
	raw = replacer.Replace(raw)
	parts := strings.Fields(raw)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) >= 4 {
			out = append(out, part)
		}
	}
	return dedupeStrings(out)
}

func toEngineTests(items []modelTestResult) []engine.TestResult {
	out := make([]engine.TestResult, 0, len(items))
	for _, item := range items {
		out = append(out, engine.TestResult{Name: item.Name, Result: item.Result})
	}
	return out
}

func mapReportStatus(status string) engine.ReportStatus {
	switch strings.TrimSpace(status) {
	case string(engine.ReportDone):
		return engine.ReportDone
	case string(engine.ReportBlocked):
		return engine.ReportBlocked
	case string(engine.ReportFailed):
		return engine.ReportFailed
	default:
		return engine.ReportPartial
	}
}

func limitStrings(items []string, max int) []string {
	if max <= 0 || len(items) <= max {
		return items
	}
	return append([]string{}, items[:max]...)
}

func emitActivity(fn engine.ActivityFunc, a engine.Activity) {
	if fn == nil {
		return
	}
	fn(a)
}

func rememberToolCall(remember engine.MemoryFunc, agentID, phase, ticketID string, record engine.ToolCall, result string) {
	if remember == nil {
		return
	}
	target := strings.TrimSpace(record.Target)
	content := strings.TrimSpace(record.Name)
	if target != "" {
		content += " " + target
	}
	if strings.TrimSpace(record.Status) != "" {
		content += " [" + record.Status + "]"
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

// describeToolCall returns (toolName, target, detail) for the live activity
// strip. It best-effort decodes the model's tool arguments to surface the
// human-meaningful target — e.g. "main.go" for file_read, the regex pattern
// for grep — without ever hard-failing if the JSON is unexpected.
func describeToolCall(call chatprovider.ToolCall) (string, string, string) {
	name := strings.TrimSpace(call.Function.Name)
	args := strings.TrimSpace(call.Function.Arguments)
	if args == "" {
		return name, "", actionVerb(name)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return name, "", actionVerb(name)
	}
	target := ""
	switch name {
	case "file_read":
		target = stringVal(raw, "filePath")
		offset := intVal(raw, "offset")
		limit := intVal(raw, "limit")
		detail := fmt.Sprintf("Reading %s", target)
		if limit > 0 {
			start := offset
			if start < 1 {
				start = 1
			}
			detail = fmt.Sprintf("Reading %s · L%d-%d", target, start, start+limit-1)
		}
		return name, target, detail
	case "file_edit":
		target = stringVal(raw, "filePath")
		return name, target, fmt.Sprintf("Editing %s", target)
	case "file_write":
		target = stringVal(raw, "filePath")
		return name, target, fmt.Sprintf("Writing new %s", target)
	case "file_delete":
		target = stringVal(raw, "filePath")
		return name, target, fmt.Sprintf("Deleting %s", target)
	case "list":
		target = stringVal(raw, "path")
		if target == "" {
			target = "."
		}
		return name, target, fmt.Sprintf("Listing %s", target)
	case "glob":
		target = stringVal(raw, "pattern")
		return name, target, fmt.Sprintf("Globbing %s", target)
	case "grep":
		target = stringVal(raw, "pattern")
		return name, target, fmt.Sprintf("Searching for %q", target)
	case "guidance_list":
		return name, "guidance", "Listing guidance"
	case "guidance_get":
		target = strings.Join(stringSliceVal(raw, "ids"), ",")
		if target == "" {
			target = "guidance"
		}
		return name, target, fmt.Sprintf("Reading guidance: %s", target)
	case "complete":
		target = stringVal(raw, "status")
		return name, target, "Completing ticket"
	case "shell":
		target = stringVal(raw, "command")
		return name, target, fmt.Sprintf("Running: %s", target)
	default:
		return name, "", actionVerb(name)
	}
}

func actionVerb(tool string) string {
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
	case "file_edit":
		return "Editing"
	case "file_write":
		return "Writing"
	case "file_delete":
		return "Deleting"
	case "complete":
		return "Completing ticket"
	case "shell":
		return "Running command"
	default:
		if tool == "" {
			return "Working"
		}
		return tool
	}
}

func stringVal(m map[string]any, key string) string {
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

func stringSliceVal(m map[string]any, key string) []string {
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

func intVal(m map[string]any, key string) int {
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
