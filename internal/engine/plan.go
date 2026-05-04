package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) RunPlan(ctx context.Context, workspacePath, userMessage string, cmRunner CMRunner, emit func(Event), reportActivity ActivityFunc) error {
	if strings.TrimSpace(workspacePath) == "" {
		return fmt.Errorf("workspace path is empty")
	}
	if emit == nil {
		emit = func(Event) {}
	}
	if reportActivity == nil {
		reportActivity = func(Activity) {}
	}

	bootstrap, err := s.LoadWorkspace(workspacePath)
	if err != nil {
		return err
	}
	session := bootstrap.Session
	session.Mode = SessionModePlan
	session.State = StatePlanning
	session.AutonomyMode = AutonomyWaitingOnUser
	session.ActiveAgentID = nil
	session.ActiveTicketID = nil

	cm := firstActiveCM(bootstrap.Roster)
	session.ActiveCMID = strPtr(cm.ID)
	if err := s.SaveSession(workspacePath, session); err != nil {
		return err
	}

	if err := s.AppendConversation(workspacePath, ConversationEntry{
		Timestamp: s.now(),
		Mode:      SessionModePlan,
		Role:      "user",
		Content:   strings.TrimSpace(userMessage),
	}); err != nil {
		return err
	}

	history, _ := s.LoadConversation(workspacePath)
	eventID := workflowEventID(session.SessionID, cm.ID, "plan", "", len(history))
	emit(Event{
		ID:      eventID,
		Role:    "contre-maitre",
		ActorID: cm.ID,
		Phase:   "plan",
		Status:  EventRunning,
		Content: "Reading the repo and answering in plan mode.",
	})

	result, err := cmRunner.RunPlan(ctx, CMPlanRequest{
		Session:       session,
		CM:            cm,
		WorkspacePath: workspacePath,
		UserMessage:   userMessage,
		History:       history,
		Gate:          s.gate,
		Activity: func(a Activity) {
			if a.Role == "" {
				a.Role = "contre-maitre"
			}
			if a.Actor == "" {
				a.Actor = cm.ID
			}
			if a.Phase == "" {
				a.Phase = "plan"
			}
			reportActivity(a)
		},
	})
	if err != nil {
		return err
	}
	answer := strings.TrimSpace(result.Answer)
	if answer == "" {
		answer = strings.TrimSpace(result.Summary)
	}
	if answer == "" {
		answer = "Plan mode completed without a response."
	}

	if err := s.AppendConversation(workspacePath, ConversationEntry{
		Timestamp: s.now(),
		Mode:      SessionModePlan,
		Role:      "contre-maitre",
		ActorID:   cm.ID,
		Content:   answer,
	}); err != nil {
		return err
	}
	history, _ = s.LoadConversation(workspacePath)
	if err := s.UpdatePlanArtifact(workspacePath, session, result, history); err != nil {
		return err
	}

	session.State = StateDone
	session.AutonomyMode = AutonomyWaitingOnUser
	session.ActiveCMID = strPtr(cm.ID)
	if err := s.SaveSession(workspacePath, session); err != nil {
		return err
	}
	emit(Event{
		ID:      eventID,
		Role:    "contre-maitre",
		ActorID: cm.ID,
		Phase:   "plan",
		Status:  EventDone,
		Content: answer,
	})
	return nil
}

func (s *Service) AppendConversation(workspacePath string, entry ConversationEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = s.now()
	}
	if entry.Mode == "" {
		entry.Mode = SessionModePlan
	}
	entry.Content = strings.TrimSpace(entry.Content)
	if entry.Content == "" {
		return nil
	}
	return appendJSONLine(filepath.Join(workspacePath, conversationFileName), entry)
}

func (s *Service) LoadConversation(workspacePath string) ([]ConversationEntry, error) {
	path := filepath.Join(workspacePath, conversationFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	entries := make([]ConversationEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry ConversationEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("decode conversation line: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *Service) UpdatePlanArtifact(workspacePath string, session SessionFile, result CMPlanResult, history []ConversationEntry) error {
	lines := []string{
		"# Plan Memory",
		"",
		fmt.Sprintf("- Session ID: %s", session.SessionID),
		fmt.Sprintf("- Last Updated: %s", s.now().Format(time.RFC3339)),
		"",
		"## User Goal",
		fmt.Sprintf("- %s", session.UserRequest),
		"",
		"## Conversation Summary",
	}
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		summary = summarizeConversationForPlan(history)
	}
	lines = append(lines, ensureBullet(summary), "", "## Latest Contre-Maitre Answer")
	answer := strings.TrimSpace(result.Answer)
	if answer == "" {
		answer = strings.TrimSpace(result.Summary)
	}
	for _, line := range strings.Split(answer, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, ensureBullet(line))
		}
	}
	lines = append(lines, "", "## Recent Conversation")
	for _, item := range tailConversation(history, 12) {
		actor := item.Role
		if item.ActorID != "" {
			actor += "/" + item.ActorID
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", actor, compactLine(item.Content, 500)))
	}
	lines = append(lines, "")
	return writeText(filepath.Join(workspacePath, planFileName), strings.Join(lines, "\n"))
}

func PlanContextForPrompt(workspacePath string) string {
	parts := []string{}
	if data, err := os.ReadFile(filepath.Join(workspacePath, planFileName)); err == nil && strings.TrimSpace(string(data)) != "" {
		parts = append(parts, "Plan memory:\n"+limitText(string(data), 12000))
	}
	if data, err := os.ReadFile(filepath.Join(workspacePath, conversationFileName)); err == nil && strings.TrimSpace(string(data)) != "" {
		parts = append(parts, "Conversation JSONL:\n"+limitText(renderConversationJSONLForPrompt(string(data)), 12000))
	}
	if len(parts) == 0 {
		return "No prior plan-mode conversation has been recorded."
	}
	return strings.Join(parts, "\n\n")
}

func firstActiveCM(roster RosterFile) AgentSpec {
	for _, agent := range roster.CMAgents {
		if agent.Active {
			return agent
		}
	}
	if len(roster.CMAgents) > 0 {
		return roster.CMAgents[0]
	}
	return AgentSpec{ID: "cm-01", Role: "cm_initial", Active: true}
}

func summarizeConversationForPlan(history []ConversationEntry) string {
	if len(history) == 0 {
		return "No plan conversation yet."
	}
	last := history[len(history)-1]
	return fmt.Sprintf("Latest %s message: %s", last.Role, compactLine(last.Content, 600))
}

func tailConversation(history []ConversationEntry, n int) []ConversationEntry {
	if n <= 0 || len(history) <= n {
		return history
	}
	return history[len(history)-n:]
}

func compactLine(s string, maxLen int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if maxLen > 0 && len(s) > maxLen {
		return strings.TrimSpace(s[:maxLen]) + "..."
	}
	return s
}

func limitText(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if maxLen > 0 && len(s) > maxLen {
		return s[:maxLen] + "\n...truncated..."
	}
	return s
}

func renderConversationJSONLForPrompt(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
