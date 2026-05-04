package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrAwaitingUserInput = fmt.Errorf("awaiting user input")

func (s *Service) LoadWorkspace(workspacePath string) (*BootstrapResult, error) {
	session, err := s.loadSession(workspacePath)
	if err != nil {
		return nil, err
	}
	roster, err := s.loadRoster(workspacePath)
	if err != nil {
		return nil, err
	}
	backlog, err := s.loadBacklog(workspacePath)
	if err != nil {
		return nil, err
	}
	return &BootstrapResult{
		Session:       session,
		Roster:        roster,
		Backlog:       backlog,
		WorkspacePath: workspacePath,
	}, nil
}

func (s *Service) loadSession(workspacePath string) (SessionFile, error) {
	var session SessionFile
	if err := readJSON(filepath.Join(workspacePath, sessionFileName), &session); err != nil {
		return SessionFile{}, err
	}
	return session, nil
}

func (s *Service) loadRoster(workspacePath string) (RosterFile, error) {
	var roster RosterFile
	if err := readJSON(filepath.Join(workspacePath, rosterFileName), &roster); err != nil {
		return RosterFile{}, err
	}
	return roster, nil
}

func (s *Service) loadBacklog(workspacePath string) (BacklogFile, error) {
	var backlog BacklogFile
	if err := readJSON(filepath.Join(workspacePath, backlogFileName), &backlog); err != nil {
		return BacklogFile{}, err
	}
	return backlog, nil
}

func (s *Service) SaveSession(workspacePath string, session SessionFile) error {
	session.UpdatedAt = s.now()
	return writeJSON(filepath.Join(workspacePath, sessionFileName), session)
}

func (s *Service) SaveBacklog(workspacePath string, backlog BacklogFile) error {
	return writeJSON(filepath.Join(workspacePath, backlogFileName), backlog)
}

func (s *Service) SubmitQuestionAnswers(workspacePath string, answers []string) (SessionFile, error) {
	session, err := s.loadSession(workspacePath)
	if err != nil {
		return SessionFile{}, err
	}
	if !session.QuestionGate.Asked || !session.QuestionGate.BlockedOnUser {
		return SessionFile{}, fmt.Errorf("session is not waiting for question answers")
	}
	if len(answers) != len(session.QuestionBatch.Questions) {
		return SessionFile{}, fmt.Errorf("expected %d answers, got %d", len(session.QuestionBatch.Questions), len(answers))
	}

	batch := session.QuestionBatch
	batch.Answers = make([]QuestionAnswer, 0, len(batch.Questions))
	for i, q := range batch.Questions {
		answer := strings.TrimSpace(answers[i])
		if answer == "" {
			return SessionFile{}, fmt.Errorf("answer %d is empty", i+1)
		}
		batch.Answers = append(batch.Answers, QuestionAnswer{
			QuestionID: q.ID,
			Answer:     answer,
		})
	}

	session.QuestionBatch = batch
	session.QuestionGate.Answered = true
	session.QuestionGate.BlockedOnUser = false
	session.AutonomyMode = AutonomyAutonomous
	session.State = StatePlanning
	if err := s.SaveSession(workspacePath, session); err != nil {
		return SessionFile{}, err
	}
	if err := s.writeQuestionsArtifact(workspacePath, session); err != nil {
		return SessionFile{}, err
	}

	return session, nil
}

func (s *Service) writeQuestionsArtifact(workspacePath string, session SessionFile) error {
	return writeText(filepath.Join(workspacePath, questionsFileName), renderQuestionsState(session))
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %q: %w", path, err)
	}
	return nil
}

func renderQuestionsState(session SessionFile) string {
	lines := []string{
		"# Questions",
		"",
		"## Need For Questions",
		fmt.Sprintf("- Needed: %t", session.QuestionGate.Needed),
		fmt.Sprintf("- Asked by: %s", session.QuestionBatch.AskedBy),
		fmt.Sprintf("- Question count: %d", session.QuestionGate.QuestionCount),
		"",
		"## User Questions",
	}

	if len(session.QuestionBatch.Questions) == 0 {
		lines = append(lines, "No clarifying questions were required.")
	} else {
		for _, q := range session.QuestionBatch.Questions {
			lines = append(lines, fmt.Sprintf("- [%s] %s", q.ID, q.Prompt))
			if strings.TrimSpace(q.Rationale) != "" {
				lines = append(lines, fmt.Sprintf("  rationale: %s", q.Rationale))
			}
		}
	}

	lines = append(lines, "", "## User Answers")
	if len(session.QuestionBatch.Answers) == 0 {
		if session.QuestionGate.BlockedOnUser {
			lines = append(lines, "Pending user answers.")
		} else {
			lines = append(lines, "No answers required.")
		}
	} else {
		for _, answer := range session.QuestionBatch.Answers {
			lines = append(lines, fmt.Sprintf("- [%s] %s", answer.QuestionID, answer.Answer))
		}
	}

	lines = append(lines, "", "## Locked Decisions")
	if len(session.QuestionBatch.LockedDecisions) == 0 {
		lines = append(lines, "- None locked yet.")
	} else {
		for _, item := range session.QuestionBatch.LockedDecisions {
			lines = append(lines, ensureBullet(item))
		}
	}

	lines = append(lines, "", "## Assumptions")
	if len(session.QuestionBatch.Assumptions) == 0 {
		lines = append(lines, "- None recorded yet.")
	} else {
		for _, item := range session.QuestionBatch.Assumptions {
			lines = append(lines, ensureBullet(item))
		}
	}

	lines = append(lines, "")
	return strings.Join(lines, "\n")
}
