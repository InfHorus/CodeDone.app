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

func (s *Service) RunSession(ctx context.Context, workspacePath string, cmRunner CMRunner, runner ImplementerRunner, testRunner TestRunner, commitFn SessionCommitFunc, emit func(Event), reportActivity ActivityFunc) error {
	if strings.TrimSpace(workspacePath) == "" {
		return fmt.Errorf("workspace path is empty")
	}
	if emit == nil {
		emit = func(Event) {}
	}
	if reportActivity == nil {
		reportActivity = func(Activity) {}
	}
	stamp := func(role, actor, phase, ticketID string) ActivityFunc {
		return func(a Activity) {
			if a.Role == "" {
				a.Role = role
			}
			if a.Actor == "" {
				a.Actor = actor
			}
			if a.Phase == "" {
				a.Phase = phase
			}
			if a.TicketID == "" {
				a.TicketID = ticketID
			}
			reportActivity(a)
		}
	}

	bootstrap, err := s.LoadWorkspace(workspacePath)
	if err != nil {
		return err
	}
	sessionPath := filepath.Join(workspacePath, sessionFileName)
	session := bootstrap.Session
	roster := bootstrap.Roster
	backlog := bootstrap.Backlog

	cm1 := roster.CMAgents[0].ID
	finalCM := roster.CMAgents[len(roster.CMAgents)-1].ID

	if session.QuestionGate.DecisionPending {
		questionGateEventID := workflowEventID(session.SessionID, cm1, "question-gate", "", 0)
		session.ActiveCMID = strPtr(cm1)
		session.State = StateQuestionGate
		session.AutonomyMode = AutonomyQuestionGate
		session.UpdatedAt = s.now()
		if err := writeJSON(sessionPath, session); err != nil {
			return err
		}
		emit(Event{
			ID:      questionGateEventID,
			Role:    "contre-maitre",
			ActorID: cm1,
			Phase:   "question gate",
			Status:  EventRunning,
			Content: "Reviewing the initial request and deciding whether clarification is required before autonomous planning.",
		})

		decision, err := runQuestionGate(ctx, cmRunner, CMQuestionGateRequest{
			Session:       session,
			CM:            roster.CMAgents[0],
			WorkspacePath: workspacePath,
			Activity:      stamp("contre-maitre", cm1, "question gate", ""),
			Gate:          s.gate,
		})
		if err != nil {
			return err
		}

		session.QuestionGate.DecisionPending = false
		session.QuestionGate.Needed = decision.Needed
		session.QuestionGate.Asked = decision.Needed
		session.QuestionGate.QuestionCount = len(decision.Questions)
		session.QuestionGate.Answered = !decision.Needed
		session.QuestionGate.BlockedOnUser = decision.Needed
		session.QuestionBatch = QuestionBatch{
			AskedBy:         cm1,
			Summary:         decision.Summary,
			Questions:       decision.Questions,
			Answers:         nil,
			LockedDecisions: decision.LockedDecisions,
			Assumptions:     decision.Assumptions,
		}
		if decision.Needed {
			session.State = StateBlocked
			session.AutonomyMode = AutonomyWaitingOnUser
			session.ActiveAgentID = nil
			session.ActiveTicketID = nil
			if err := s.SaveSession(workspacePath, session); err != nil {
				return err
			}
			if err := s.writeQuestionsArtifact(workspacePath, session); err != nil {
				return err
			}
			emit(Event{
				ID:      questionGateEventID,
				Role:    "contre-maitre",
				ActorID: cm1,
				Phase:   "question gate",
				Status:  EventWaiting,
				Content: formatQuestionGateMessage(session.QuestionBatch),
			})
			return ErrAwaitingUserInput
		}

		session.State = StatePlanning
		session.AutonomyMode = AutonomyAutonomous
		if err := s.SaveSession(workspacePath, session); err != nil {
			return err
		}
		if err := s.writeQuestionsArtifact(workspacePath, session); err != nil {
			return err
		}
		emit(Event{
			ID:      questionGateEventID,
			Role:    "contre-maitre",
			ActorID: cm1,
			Phase:   "question gate",
			Status:  EventDone,
			Content: decision.Summary,
		})
		if err := s.gateWait(ctx, 120*time.Millisecond); err != nil {
			return err
		}
	} else if session.QuestionGate.Asked && !session.QuestionGate.Answered {
		return ErrAwaitingUserInput
	}

	if !backlog.Frozen {
		existingDirective, _ := s.LoadDirective(workspacePath)
		previousCMID := ""
		for i, cmAgent := range roster.CMAgents {
			directiveEventID := workflowEventID(session.SessionID, cmAgent.ID, "directive-pass", "", versionForIndex(i))
			session.ActiveCMID = strPtr(cmAgent.ID)
			if i == 0 && len(roster.CMAgents) == 1 {
				session.State = StatePlanning
			} else if i == 0 {
				session.State = StatePlanning
			} else {
				session.State = StateDirectiveReview
			}
			if err := s.SaveSession(workspacePath, session); err != nil {
				return err
			}
			emit(Event{
				ID:      directiveEventID,
				Role:    "contre-maitre",
				ActorID: cmAgent.ID,
				Phase:   "directive pass",
				Status:  EventRunning,
				Content: directiveStartMessage(i, len(roster.CMAgents)),
			})

			version := i + 1
			result, err := runDirectivePass(ctx, cmRunner, CMDirectiveRequest{
				Session:           session,
				CM:                cmAgent,
				PreviousCMID:      previousCMID,
				WorkspacePath:     workspacePath,
				Version:           version,
				Phase:             directivePhase(i, len(roster.CMAgents)),
				ExistingDirective: existingDirective,
				Activity:          stamp("contre-maitre", cmAgent.ID, "directive pass", ""),
				Gate:              s.gate,
			})
			if err != nil {
				return err
			}

			session.UpdatedAt = s.now()
			nextDoc := DirectiveDocument{
				SessionID:       session.SessionID,
				CurrentCMOwner:  cmAgent.ID,
				PreviousCMOwner: previousCMID,
				Version:         version,
				LastUpdated:     session.UpdatedAt,
				Sections:        result.Sections,
			}
			if err := s.UpdateDirective(workspacePath, nextDoc); err != nil {
				return err
			}
			existingDirective = nextDoc
			previousCMID = cmAgent.ID
			emit(Event{
				ID:      directiveEventID,
				Role:    "contre-maitre",
				ActorID: cmAgent.ID,
				Phase:   "directive pass",
				Status:  EventDone,
				Content: result.Summary,
			})
			if err := s.gateWait(ctx, 120*time.Millisecond); err != nil {
				return err
			}
		}

		backlogEventID := workflowEventID(session.SessionID, finalCM, "backlog", "", 0)
		emit(Event{
			ID:      backlogEventID,
			Role:    "contre-maitre",
			ActorID: finalCM,
			Phase:   "backlog planning",
			Status:  EventRunning,
			Content: "Freezing the directive and converting it into an atomic serialized backlog.",
		})
		var tickets []TicketFile
		backlog, tickets, err = s.BuildBacklog(ctx, workspacePath, session, roster.CMAgents[len(roster.CMAgents)-1], roster.ImplementerAgents, cmRunner, stamp("contre-maitre", finalCM, "backlog planning", ""))
		if err != nil {
			return err
		}

		session.ActiveCMID = strPtr(finalCM)
		session.State = StateBacklogFrozen
		session.ActiveAgentID = nil
		session.ActiveTicketID = nil
		session.BacklogSummary = summarizeBacklog(backlog, tickets)
		if err := s.SaveSession(workspacePath, session); err != nil {
			return err
		}
		emit(Event{
			ID:      backlogEventID,
			Role:    "contre-maitre",
			ActorID: finalCM,
			Phase:   "backlog planning",
			Status:  EventDone,
			Content: fmt.Sprintf("Final CM froze planning and generated the initial backlog: %d tickets are ready for serialized dispatch. Review splits may append follow-up tickets later.", len(tickets)),
		})
		if err := s.gateWait(ctx, 120*time.Millisecond); err != nil {
			return err
		}
	}

	tickets, err := loadTickets(workspacePath, backlog.TicketOrder)
	if err != nil {
		return err
	}

	session.State = StateDispatchReady
	if err := s.SaveSession(workspacePath, session); err != nil {
		return err
	}

	for _, ticket := range tickets {
		if ticket.ScopeClass == "stretch" && ticket.Status == TicketTodo {
			entry := ReviewLogEntry{
				Timestamp:  s.now(),
				ReviewerID: finalCM,
				TicketID:   ticket.ID,
				Attempt:    0,
				Decision:   DecisionDefer,
				Reason:     "Stretch-scope work deferred until required backlog is complete.",
				NextAction: ReviewNextAction{Type: "defer_ticket"},
			}
			backlog, session, err = s.deferTicket(workspacePath, backlog, session, ticket.ID, finalCM)
			if err != nil {
				return err
			}
			if err := appendJSONLine(filepath.Join(workspacePath, reviewLogFileName), entry); err != nil {
				return err
			}
			emit(Event{
				ID:       workflowEventID(session.SessionID, finalCM, "review", ticket.ID, 0),
				Role:     "contre-maitre",
				ActorID:  finalCM,
				Phase:    "review",
				TicketID: ticket.ID,
				Status:   EventDone,
				Content:  fmt.Sprintf("Deferred stretch ticket %s until the required backlog is complete.", ticket.ID),
			})
		}
	}

	attempts := map[string]int{}
	assignments := map[string]int{}
	// Snapshot the dispatch order *before* the loop runs so the displayed
	// "ticket X of N" stays stable across revises/splits. Splits append child
	// tickets later — those get an extended numbering after the original set.
	originalTotal := countDispatchableTickets(tickets)
	for i := 0; i < len(tickets); i++ {
		ticket, err := loadTicket(filepath.Join(workspacePath, "tickets", tickets[i].ID+".json"))
		if err != nil {
			return err
		}
		if ticket.Status != TicketTodo {
			continue
		}
		implementer := chooseImplementerForTicket(roster.ImplementerAgents, ticket.SuggestedProfile, assignments)
		assignments[implementer.ID]++
		backlog, session, err := s.startTicket(workspacePath, backlog, session, ticket.ID, finalCM, implementer.ID)
		if err != nil {
			return err
		}
		ticketIdx, ticketTotal := ticketDispatchPosition(tickets, i, originalTotal)
		emit(Event{
			ID:          workflowEventID(session.SessionID, finalCM, "dispatch", ticket.ID, attempts[ticket.ID]+1),
			Role:        "contre-maitre",
			ActorID:     finalCM,
			Phase:       "dispatch",
			TicketID:    ticket.ID,
			Status:      EventDone,
			Content:     fmt.Sprintf("Dispatching %s to %s.", ticket.ID, implementer.ID),
			TicketIndex: ticketIdx,
			TicketTotal: ticketTotal,
			TicketTitle: ticket.Title,
		})
		if err := s.gateWait(ctx, 120*time.Millisecond); err != nil {
			return err
		}

		attemptSnapshot, err := snapshotRepoFiles(session.UserRepoPath)
		if err != nil {
			return err
		}

		attempts[ticket.ID]++
		implementerEventID := workflowEventID(session.SessionID, implementer.ID, "implement", ticket.ID, attempts[ticket.ID])
		emit(Event{
			ID:          implementerEventID,
			Role:        "implementer",
			ActorID:     implementer.ID,
			Phase:       "implementing",
			TicketID:    ticket.ID,
			Status:      EventRunning,
			Content:     fmt.Sprintf("Working on %s: %s", ticket.ID, ticket.Title),
			TicketIndex: ticketIdx,
			TicketTotal: ticketTotal,
			TicketTitle: ticket.Title,
		})
		runResult, err := runImplementer(ctx, runner, ImplementerRunRequest{
			Session:       session,
			Ticket:        ticket,
			Implementer:   implementer,
			Attempt:       attempts[ticket.ID],
			WorkspacePath: workspacePath,
			RepoPath:      session.UserRepoPath,
			Activity:      stamp("implementer", implementer.ID, "implementing", ticket.ID),
		})
		if err != nil {
			return err
		}
		if testRunner != nil {
			tests, testErr := testRunner.Run(ctx, TestRunRequest{
				RepoPath:      session.UserRepoPath,
				WorkspacePath: workspacePath,
				Session:       session,
				Ticket:        ticket,
				ChangedFiles:  runResult.FilesChanged,
			})
			runResult.TestsRun = tests
			if testErr != nil {
				runResult.Status = ReportPartial
				runResult.KnownRisks = append(runResult.KnownRisks, "Engine-owned test execution failed: "+testErr.Error())
			}
			if hasFailingTests(tests) && runResult.Status == ReportDone {
				runResult.Status = ReportPartial
				runResult.KnownRisks = append(runResult.KnownRisks, "One or more engine-owned test commands failed.")
			}
		}
		report := buildReportFromRunResult(session.SessionID, ticket, implementer, attempts[ticket.ID], s.now(), runResult)
		if err := writeJSON(filepath.Join(workspacePath, "reports", fmt.Sprintf("%s.attempt-%d.json", ticket.ID, attempts[ticket.ID])), report); err != nil {
			return err
		}
		for idx, prev := range runResult.FilePreviews {
			payload, mErr := json.Marshal(prev)
			if mErr != nil {
				continue
			}
			emit(Event{
				ID:          fmt.Sprintf("%s.diff-%d", implementerEventID, idx),
				Role:        "implementer",
				ActorID:     implementer.ID,
				Phase:       "file_diff",
				TicketID:    ticket.ID,
				Status:      EventDone,
				Content:     string(payload),
				TicketIndex: ticketIdx,
				TicketTotal: ticketTotal,
				TicketTitle: ticket.Title,
			})
		}
		emit(Event{
			ID:          implementerEventID,
			Role:        "implementer",
			ActorID:     implementer.ID,
			Phase:       "implementing",
			TicketID:    ticket.ID,
			Status:      EventDone,
			Content:     formatImplementerRecap(ticket, report),
			TicketIndex: ticketIdx,
			TicketTotal: ticketTotal,
			TicketTitle: ticket.Title,
		})
		if err := s.gateWait(ctx, 120*time.Millisecond); err != nil {
			return err
		}

		reviewEventID := workflowEventID(session.SessionID, finalCM, "review", ticket.ID, attempts[ticket.ID])
		emit(Event{
			ID:       reviewEventID,
			Role:     "contre-maitre",
			ActorID:  finalCM,
			Phase:    "review",
			TicketID: ticket.ID,
			Status:   EventRunning,
			Content:  fmt.Sprintf("Reviewing %s against its ticket, diff, and test results.", ticket.ID),
		})
		review, err := runCMReview(ctx, cmRunner, CMReviewRequest{
			Session:       session,
			CM:            roster.CMAgents[len(roster.CMAgents)-1],
			WorkspacePath: workspacePath,
			Ticket:        ticket,
			Report:        report,
			Backlog:       backlog,
			AutoCommit:    session.AutoCommit,
			Activity:      stamp("contre-maitre", finalCM, "review", ticket.ID),
		})
		if err != nil {
			return err
		}
		if strings.TrimSpace(review.Reason) == "" {
			review.Reason = "Review decision returned without an explicit reason."
		}
		if review.Decision == DecisionRevise && shouldAutoBlockAfterRetries(report, attempts[ticket.ID]) {
			review.Decision = DecisionBlock
			review.Summary = fmt.Sprintf("%s\n\nTicket auto-blocked after %d CM revision attempts.", strings.TrimSpace(review.Summary), attempts[ticket.ID])
			review.Reason = autoBlockReason(report, review.Reason, attempts[ticket.ID])
		}
		nextAction := reviewNextAction(review.Decision, ticket.ID, implementer.ID, backlog, roster.ImplementerAgents, workspacePath)
		entry := ReviewLogEntry{
			Timestamp:     s.now(),
			ReviewerID:    finalCM,
			TicketID:      ticket.ID,
			Attempt:       attempts[ticket.ID],
			Decision:      review.Decision,
			Reason:        review.Reason,
			GitDiffRef:    report.GitDiffRef,
			GitDiffStat:   report.GitDiffStat,
			NextAction:    nextAction,
			FollowupNotes: review.FollowupNotes,
		}

		switch review.Decision {
		case DecisionSplit:
			if err := restoreAttemptFiles(session.UserRepoPath, attemptSnapshot, report.FilesChanged); err != nil {
				return err
			}
			children, reviewErr := func() ([]TicketFile, error) {
				var splitChildren []TicketFile
				backlog, session, splitChildren, err = s.splitTicketWithBlueprints(workspacePath, backlog, session, ticket.ID, finalCM, review.ChildTickets)
				return splitChildren, err
			}()
			if reviewErr != nil {
				return reviewErr
			}
			tickets = append(tickets, children...)
			if err := appendJSONLine(filepath.Join(workspacePath, reviewLogFileName), entry); err != nil {
				return err
			}
			emit(Event{
				ID:       reviewEventID,
				Role:     "contre-maitre",
				ActorID:  finalCM,
				Phase:    "review",
				TicketID: ticket.ID,
				Status:   EventDone,
				Content:  formatReviewSummary(ticket, review.Summary, fmt.Sprintf("Final CM split %s after review and generated child tickets.", ticket.ID)),
			})
		case DecisionRevise:
			if err := restoreAttemptFiles(session.UserRepoPath, attemptSnapshot, report.FilesChanged); err != nil {
				return err
			}
			backlog, session, err = s.reviseTicket(workspacePath, backlog, session, ticket.ID, finalCM)
			if err != nil {
				return err
			}
			if err := appendJSONLine(filepath.Join(workspacePath, reviewLogFileName), entry); err != nil {
				return err
			}
			emit(Event{
				ID:       reviewEventID,
				Role:     "contre-maitre",
				ActorID:  finalCM,
				Phase:    "review",
				TicketID: ticket.ID,
				Status:   EventDone,
				Content:  formatReviewSummary(ticket, review.Summary, fmt.Sprintf("Final CM requested a revision on %s. Ticket returned to the ready queue.", ticket.ID)),
			})
			i--
		case DecisionDefer:
			if err := restoreAttemptFiles(session.UserRepoPath, attemptSnapshot, report.FilesChanged); err != nil {
				return err
			}
			backlog, session, err = s.deferTicket(workspacePath, backlog, session, ticket.ID, finalCM)
			if err != nil {
				return err
			}
			if err := appendJSONLine(filepath.Join(workspacePath, reviewLogFileName), entry); err != nil {
				return err
			}
			emit(Event{
				ID:       reviewEventID,
				Role:     "contre-maitre",
				ActorID:  finalCM,
				Phase:    "review",
				TicketID: ticket.ID,
				Status:   EventDone,
				Content:  formatReviewSummary(ticket, review.Summary, fmt.Sprintf("Final CM deferred %s after review.", ticket.ID)),
			})
		case DecisionBlock:
			if err := restoreAttemptFiles(session.UserRepoPath, attemptSnapshot, report.FilesChanged); err != nil {
				return err
			}
			backlog, session, err = s.blockTicket(workspacePath, backlog, session, ticket.ID, finalCM)
			if err != nil {
				return err
			}
			if err := appendJSONLine(filepath.Join(workspacePath, reviewLogFileName), entry); err != nil {
				return err
			}
			emit(Event{
				ID:       reviewEventID,
				Role:     "contre-maitre",
				ActorID:  finalCM,
				Phase:    "review",
				TicketID: ticket.ID,
				Status:   EventDone,
				Content:  formatReviewSummary(ticket, review.Summary, fmt.Sprintf("Final CM blocked %s after review.", ticket.ID)),
			})
		default:
			if err := appendJSONLine(filepath.Join(workspacePath, reviewLogFileName), entry); err != nil {
				return err
			}
			backlog, session, err = s.acceptTicket(workspacePath, backlog, session, ticket.ID, finalCM)
			if err != nil {
				return err
			}
			emit(Event{
				ID:       reviewEventID,
				Role:     "contre-maitre",
				ActorID:  finalCM,
				Phase:    "review",
				TicketID: ticket.ID,
				Status:   EventDone,
				Content:  formatReviewSummary(ticket, review.Summary, fmt.Sprintf("Final CM accepted %s and updated the backlog.", ticket.ID)),
			})
		}
		if err := s.gateWait(ctx, 120*time.Millisecond); err != nil {
			return err
		}
	}

	autoCommit := AutoCommitOutcome{Enabled: session.AutoCommit}
	if session.AutoCommit && commitFn != nil {
		commitEventID := workflowEventID(session.SessionID, "engine", "auto-commit", "", 0)
		emit(Event{
			ID:      commitEventID,
			Role:    "system",
			ActorID: "engine",
			Phase:   "auto-commit",
			Status:  EventRunning,
			Content: "Auto-commit is enabled. Staging and committing all session changes.",
		})
		message := fmt.Sprintf("CodeDone session %s\n\n%s", session.SessionID, strings.TrimSpace(session.UserRequest))
		autoCommit.Message = message
		sha, commitErr := commitFn(session.UserRepoPath, message)
		if commitErr != nil {
			autoCommit.Error = commitErr.Error()
			emit(Event{
				ID:      commitEventID,
				Role:    "system",
				ActorID: "engine",
				Phase:   "auto-commit",
				Status:  EventDone,
				Content: fmt.Sprintf("Auto-commit failed: %s. Continuing to finalizer.", commitErr.Error()),
			})
		} else if sha == "" {
			emit(Event{
				ID:      commitEventID,
				Role:    "system",
				ActorID: "engine",
				Phase:   "auto-commit",
				Status:  EventDone,
				Content: "Auto-commit had nothing to commit (clean working tree).",
			})
		} else {
			autoCommit.Performed = true
			autoCommit.CommitSHA = sha
			session.CommitSHA = sha
			emit(Event{
				ID:      commitEventID,
				Role:    "system",
				ActorID: "engine",
				Phase:   "auto-commit",
				Status:  EventDone,
				Content: fmt.Sprintf("Auto-commit recorded session changes at %s.", shortSHA(sha)),
			})
		}
	}

	session.State = StateFinalizing
	session.ActiveAgentID = strPtr("finalizer-01")
	session.ActiveTicketID = nil
	session.UpdatedAt = s.now()
	tickets, err = loadTickets(workspacePath, backlog.TicketOrder)
	if err != nil {
		return err
	}
	session.BacklogSummary = summarizeBacklog(backlog, tickets)
	finalizerEventID := workflowEventID(session.SessionID, "finalizer-01", "finalize", "", 0)
	emit(Event{
		ID:      finalizerEventID,
		Role:    "finalizer",
		ActorID: "finalizer-01",
		Phase:   "finalizing",
		Status:  EventRunning,
		Content: "Writing the final report from the completed backlog and validation results.",
	})
	if err := writeJSON(sessionPath, session); err != nil {
		return err
	}
	finalizerResult, err := runFinalizer(ctx, cmRunner, FinalizerRequest{
		Session:       session,
		WorkspacePath: workspacePath,
		Backlog:       backlog,
		Tickets:       tickets,
		AutoCommit:    autoCommit,
		Activity:      stamp("finalizer", "finalizer-01", "finalizing", ""),
	})
	if err != nil {
		return err
	}
	reportBody := strings.TrimSpace(finalizerResult.ReportBody)
	if reportBody == "" {
		reportBody = renderFinalReportResolved(session, len(tickets))
	}
	if err := writeText(filepath.Join(workspacePath, finalReportFileName), reportBody); err != nil {
		return err
	}
	emit(Event{
		ID:      finalizerEventID,
		Role:    "finalizer",
		ActorID: "finalizer-01",
		Phase:   "finalizing",
		Status:  EventDone,
		Content: coalesceSummary(finalizerResult.Summary, "Finalizer wrote the final report."),
	})

	session.State = StateDone
	session.ActiveAgentID = nil
	session.ActiveTicketID = nil
	session.BacklogSummary = summarizeBacklog(backlog, tickets)
	session.UpdatedAt = s.now()
	if err := writeJSON(sessionPath, session); err != nil {
		return err
	}

	return nil
}

func (s *Service) startTicket(workspacePath string, backlog BacklogFile, session SessionFile, ticketID, cmID, implementerID string) (BacklogFile, SessionFile, error) {
	ticket, err := loadTicket(filepath.Join(workspacePath, "tickets", ticketID+".json"))
	if err != nil {
		return backlog, session, err
	}

	ticket.Status = TicketInProgress
	ticket.AssignedTo = implementerID
	ticket.UpdatedAt = s.now()
	if err := writeJSON(filepath.Join(workspacePath, "tickets", ticket.ID+".json"), ticket); err != nil {
		return backlog, session, err
	}

	backlog.ReadyQueue = removeString(backlog.ReadyQueue, ticketID)
	session.State = StateImplementing
	session.ActiveCMID = strPtr(cmID)
	session.ActiveAgentID = strPtr(implementerID)
	session.ActiveTicketID = strPtr(ticketID)
	session.BacklogSummary = incrementBacklogState(session.BacklogSummary, TicketTodo, TicketInProgress)
	session.UpdatedAt = s.now()

	if err := writeJSON(filepath.Join(workspacePath, backlogFileName), backlog); err != nil {
		return backlog, session, err
	}
	if err := writeJSON(filepath.Join(workspacePath, sessionFileName), session); err != nil {
		return backlog, session, err
	}

	return backlog, session, nil
}

func (s *Service) acceptTicket(workspacePath string, backlog BacklogFile, session SessionFile, ticketID, cmID string) (BacklogFile, SessionFile, error) {
	ticket, err := loadTicket(filepath.Join(workspacePath, "tickets", ticketID+".json"))
	if err != nil {
		return backlog, session, err
	}

	ticket.Status = TicketDone
	ticket.UpdatedAt = s.now()
	if err := writeJSON(filepath.Join(workspacePath, "tickets", ticket.ID+".json"), ticket); err != nil {
		return backlog, session, err
	}

	backlog.DoneQueue = append(backlog.DoneQueue, ticketID)
	session.State = StateCMReview
	session.ActiveCMID = strPtr(cmID)
	session.ActiveAgentID = nil
	session.ActiveTicketID = nil
	session.BacklogSummary = incrementBacklogState(session.BacklogSummary, TicketInProgress, TicketDone)
	session.UpdatedAt = s.now()

	if err := writeJSON(filepath.Join(workspacePath, backlogFileName), backlog); err != nil {
		return backlog, session, err
	}
	if err := writeJSON(filepath.Join(workspacePath, sessionFileName), session); err != nil {
		return backlog, session, err
	}

	return backlog, session, nil
}

func buildReportFromRunResult(sessionID string, ticket TicketFile, implementer AgentSpec, attempt int, now time.Time, result ImplementerRunResult) ImplementerReport {
	return ImplementerReport{
		SessionID:          sessionID,
		TicketID:           ticket.ID,
		Attempt:            attempt,
		ImplementerID:      implementer.ID,
		StartedAt:          now.Add(-45 * time.Second),
		CompletedAt:        now,
		Status:             result.Status,
		Summary:            result.Summary,
		FilesChanged:       result.FilesChanged,
		Commits:            result.Commits,
		GitDiffRef:         result.GitDiffRef,
		GitDiffStat:        result.GitDiffStat,
		TestsRun:           result.TestsRun,
		ToolCalls:          result.ToolCalls,
		FilePreviews:       result.FilePreviews,
		ImportantDecisions: result.ImportantDecisions,
		KnownRisks:         result.KnownRisks,
		RemainingWork:      result.RemainingWork,
		SuggestedNextStep:  result.SuggestedNextStep,
	}
}

func summarizeScope(tickets []TicketFile) ScopeSummary {
	var summary ScopeSummary
	for _, ticket := range tickets {
		switch ticket.ScopeClass {
		case "required":
			summary.Required++
		case "best_in_class":
			summary.BestInClass++
		case "stretch":
			summary.Stretch++
		}
	}
	return summary
}

func summarizeBacklog(backlog BacklogFile, tickets []TicketFile) BacklogSummary {
	summary := BacklogSummary{
		Total:      len(tickets),
		Todo:       len(backlog.ReadyQueue),
		InProgress: 0,
		Done:       len(backlog.DoneQueue),
		Blocked:    len(backlog.BlockedQueue),
		Deferred:   len(backlog.DeferredQueue),
	}
	for _, ticket := range tickets {
		if ticket.Status == TicketInProgress {
			summary.InProgress++
		}
	}
	return summary
}

func incrementBacklogState(summary BacklogSummary, from, to TicketState) BacklogSummary {
	switch from {
	case TicketTodo:
		if summary.Todo > 0 {
			summary.Todo--
		}
	case TicketInProgress:
		if summary.InProgress > 0 {
			summary.InProgress--
		}
	case TicketDeferred:
		if summary.Deferred > 0 {
			summary.Deferred--
		}
	}

	switch to {
	case TicketInProgress:
		summary.InProgress++
	case TicketDone:
		summary.Done++
	case TicketTodo:
		summary.Todo++
	case TicketDeferred:
		summary.Deferred++
	}
	return summary
}

func renderQuestionsResolved(cmID string) string {
	return strings.Join([]string{
		"# Questions",
		"",
		"## Need For Questions",
		"- Needed: no",
		fmt.Sprintf("- Asked by: %s", cmID),
		"- Question count: 0",
		"",
		"## User Questions",
		"No clarifying questions were required.",
		"",
		"## User Answers",
		"No answers required.",
		"",
		"## Locked Decisions",
		"- CM1 determined the request was clear enough to proceed autonomously.",
		"",
	}, "\n")
}

func formatQuestionGateMessage(batch QuestionBatch) string {
	lines := []string{
		batch.Summary,
		"",
		"Questions for the user:",
	}
	for _, q := range batch.Questions {
		lines = append(lines, fmt.Sprintf("%s. %s", q.ID, q.Prompt))
	}
	return strings.Join(lines, "\n")
}

func directivePhase(index, total int) string {
	switch {
	case total <= 1:
		return "final"
	case index == 0:
		return "initial"
	case index == total-1:
		return "final"
	default:
		return "review"
	}
}

func runQuestionGate(ctx context.Context, runner CMRunner, req CMQuestionGateRequest) (CMQuestionGateResult, error) {
	if runner == nil {
		return CMQuestionGateResult{
			Needed:  false,
			Summary: fmt.Sprintf("%s proceeded without user questions because no CM runner was configured.", req.CM.ID),
		}, nil
	}
	return runner.RunQuestionGate(ctx, req)
}

func runDirectivePass(ctx context.Context, runner CMRunner, req CMDirectiveRequest) (CMDirectiveResult, error) {
	if runner == nil {
		return CMDirectiveResult{
			Summary: fmt.Sprintf("%s skipped directive generation because no CM runner was configured.", req.CM.ID),
			Sections: map[string][]string{
				"session_identity": {"- No CM runner configured."},
			},
		}, nil
	}
	return runner.RunDirectivePass(ctx, req)
}

func runCMReview(ctx context.Context, runner CMRunner, req CMReviewRequest) (CMReviewResult, error) {
	if runner == nil {
		return CMReviewResult{
			Summary:  fmt.Sprintf("%s accepted %s because no CM review runner was configured.", req.CM.ID, req.Ticket.ID),
			Decision: DecisionAccept,
			Reason:   "No CM review runner configured.",
		}, nil
	}
	return runner.RunReview(ctx, req)
}

func runFinalizer(ctx context.Context, runner CMRunner, req FinalizerRequest) (FinalizerResult, error) {
	if runner == nil {
		return FinalizerResult{
			Summary:    "No finalizer runner configured.",
			ReportBody: renderFinalReportResolved(req.Session, len(req.Tickets)),
		}, nil
	}
	return runner.RunFinalizer(ctx, req)
}

func loadTickets(workspacePath string, ids []string) ([]TicketFile, error) {
	tickets := make([]TicketFile, 0, len(ids))
	for _, id := range ids {
		ticket, err := loadTicket(filepath.Join(workspacePath, "tickets", id+".json"))
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}
	return tickets, nil
}

type repoFileSnapshot struct {
	Files map[string]repoFileState
}

type repoFileState struct {
	Exists bool
	Data   []byte
	Mode   os.FileMode
}

func snapshotRepoFiles(repoPath string) (repoFileSnapshot, error) {
	snapshot := repoFileSnapshot{Files: map[string]repoFileState{}}
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return snapshot, nil
	}
	root, err := filepath.Abs(repoPath)
	if err != nil {
		return snapshot, err
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot.Files[filepath.ToSlash(rel)] = repoFileState{
			Exists: true,
			Data:   data,
			Mode:   info.Mode().Perm(),
		}
		return nil
	})
	return snapshot, err
}

func restoreAttemptFiles(repoPath string, snapshot repoFileSnapshot, files []string) error {
	if strings.TrimSpace(repoPath) == "" {
		return nil
	}
	root, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, file := range files {
		rel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(file)))
		if rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return fmt.Errorf("refusing to restore unsafe repo-relative path %q", file)
		}
		key := filepath.ToSlash(rel)
		if seen[key] {
			continue
		}
		seen[key] = true
		target := filepath.Join(root, rel)
		state := snapshot.Files[key]
		if !state.Exists {
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := state.Mode
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(target, state.Data, mode); err != nil {
			return err
		}
	}
	return nil
}

func hasFailingTests(results []TestResult) bool {
	for _, result := range results {
		if strings.EqualFold(strings.TrimSpace(result.Result), "fail") {
			return true
		}
	}
	return false
}

func reviewNextAction(decision ReviewDecision, ticketID, implementerID string, backlog BacklogFile, agents []AgentSpec, workspacePath string) ReviewNextAction {
	switch decision {
	case DecisionRevise:
		return ReviewNextAction{Type: "revise_ticket", TicketID: strPtr(ticketID), ImplementerID: strPtr(implementerID)}
	case DecisionSplit:
		return ReviewNextAction{Type: "split_ticket", TicketID: strPtr(ticketID)}
	case DecisionDefer:
		return ReviewNextAction{Type: "defer_ticket", TicketID: strPtr(ticketID)}
	case DecisionBlock:
		return ReviewNextAction{Type: "block_ticket", TicketID: strPtr(ticketID)}
	default:
		nextAction := ReviewNextAction{Type: "dispatch_next_ticket"}
		if nextID, nextImpl := nextReadyTicket(backlog, agents, workspacePath); nextID != "" {
			nextAction.TicketID = strPtr(nextID)
			nextAction.ImplementerID = strPtr(nextImpl)
		}
		return nextAction
	}
}

func coalesceSummary(summary, fallback string) string {
	if strings.TrimSpace(summary) == "" {
		return fallback
	}
	return summary
}

func shouldAutoBlockAfterRetries(report ImplementerReport, attempts int) bool {
	// A completed implementation that CM keeps revising probably reflects a
	// bad ticket or unsuitable approach after three tries. Execution/tool
	// failures are different: they may include useful partial edits and should
	// get more room before the ticket is blocked.
	if report.Status == ReportDone && len(report.FilesChanged) > 0 {
		return attempts >= 3
	}
	return attempts >= 5
}

func autoBlockReason(report ImplementerReport, reviewReason string, attempts int) string {
	reviewReason = strings.TrimSpace(reviewReason)
	switch {
	case len(report.FilesChanged) == 0:
		return fmt.Sprintf("Blocked after %d attempts with no current-attempt file changes. Last review reason: %s", attempts, emptyReason(reviewReason))
	case report.Status != ReportDone:
		return fmt.Sprintf("Blocked after %d attempts because the implementer report remained %q. Last review reason: %s", attempts, report.Status, emptyReason(reviewReason))
	default:
		return fmt.Sprintf("Blocked after %d CM revision attempts. Last review reason: %s", attempts, emptyReason(reviewReason))
	}
}

func emptyReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "not provided"
	}
	return reason
}

func workflowEventID(sessionID, actorID, phase, ticketID string, attempt int) string {
	return fmt.Sprintf("%s:%s:%s:%s:%d", sessionID, actorID, phase, ticketID, attempt)
}

func versionForIndex(index int) int {
	return index + 1
}

func directiveStartMessage(index, total int) string {
	switch {
	case total <= 1:
		return "Producing the directive pass."
	case index == 0:
		return "Producing the first directive pass from the user request and answers."
	case index == total-1:
		return "Producing the final directive pass and dispatch plan."
	default:
		return "Reviewing the directive and tightening assumptions, scope, and quality bar."
	}
}

func formatImplementerRecap(ticket TicketFile, report ImplementerReport) string {
	summary := strings.TrimSpace(report.Summary)
	if summary == "" {
		summary = fmt.Sprintf("Completed %s.", ticket.ID)
	}
	files := "No files changed."
	if len(report.FilesChanged) > 0 {
		files = "Files changed: " + strings.Join(report.FilesChanged, ", ")
	}
	tools := "Tool calls: none"
	if len(report.ToolCalls) > 0 {
		parts := make([]string, 0, len(report.ToolCalls))
		for _, call := range report.ToolCalls {
			item := call.Name
			if strings.TrimSpace(call.Target) != "" {
				item += "(" + call.Target + ")"
			}
			if strings.TrimSpace(call.Status) != "" {
				item += " [" + call.Status + "]"
			}
			parts = append(parts, item)
		}
		tools = "Tool calls: " + strings.Join(parts, ", ")
	}
	return fmt.Sprintf("%s · %s\n%s\n\n%s\n%s", ticket.ID, ticket.Title, summary, files, tools)
}

func formatReviewSummary(ticket TicketFile, summary, fallback string) string {
	body := coalesceSummary(summary, fallback)
	return fmt.Sprintf("%s · %s\n%s", ticket.ID, ticket.Title, body)
}

func renderDirectiveVersion(session SessionFile, currentCMID, previousCMID string, version int, sections map[string][]string) string {
	var lines []string
	lines = append(lines,
		"# CM Directive",
		"",
		fmt.Sprintf("- Session ID: %s", session.SessionID),
		fmt.Sprintf("- Current CM Owner: %s", currentCMID),
		fmt.Sprintf("- Previous CM Owner: %s", previousCMID),
		fmt.Sprintf("- Directive Version: %d", version),
		fmt.Sprintf("- Last Updated: %s", session.UpdatedAt.Format(time.RFC3339)),
		"",
	)

	order := []struct {
		heading string
		body    []string
	}{
		{"## Session Identity", sections["session_identity"]},
		{"## User Request", sections["user_request"]},
		{"## User Answers And Locked Decisions", sections["locked_decisions"]},
		{"## Assumptions", sections["assumptions"]},
		{"## Product Framing", sections["product_framing"]},
		{"## Competitive / Reference Intelligence", sections["competitive"]},
		{"## Scope Classification", nil},
	}

	for _, item := range order {
		lines = append(lines, item.heading)
		if item.body != nil {
			lines = append(lines, item.body...)
		}
		if item.heading == "## Scope Classification" {
			lines = append(lines,
				"### Required For Category Credibility",
			)
			lines = append(lines, sections["scope_required"]...)
			lines = append(lines,
				"",
				"### Best-In-Class Enhancements",
			)
			lines = append(lines, sections["scope_best"]...)
			lines = append(lines,
				"",
				"### Stretch / Later",
			)
			lines = append(lines, sections["scope_stretch"]...)
		}
		lines = append(lines, "")
	}

	for _, heading := range []string{
		"## Architecture Direction",
		"## Full Feature Inventory",
		"## Risks And Unknowns",
		"## Backlog Strategy",
		"## Dispatch Notes For Final CM",
	} {
		key := directiveSectionKey(heading)
		lines = append(lines, heading)
		lines = append(lines, sections[key]...)
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

func renderFinalReportResolved(session SessionFile, completedTickets int) string {
	return strings.Join([]string{
		"# Final Report",
		"",
		"## Session Summary",
		fmt.Sprintf("- Session ID: %s", session.SessionID),
		"- Finalizer: finalizer-01",
		"- Final State: done",
		"",
		"## Original Goal",
		fmt.Sprintf("- %s", session.UserRequest),
		"",
		"## What Was Completed",
		"- Mock planning, backlog creation, serialized ticket dispatch, implementer reporting, and CM review persistence.",
		"",
		"## Tickets Completed",
		fmt.Sprintf("- Completed ticket count: %d", completedTickets),
		"",
		"## Deferred Or Blocked Items",
		"- None in mock flow.",
		"",
		"## Validation",
		"- Mock validation completed.",
		"- Build check represented in implementer reports.",
		"",
		"## Risks / Residual Gaps",
		"- Provider integrations are still mocked.",
		"",
		"## Final Signoff",
		"- Approved: yes",
		"- Reason: Mock session flow completed cleanly.",
		"",
	}, "\n")
}

func directiveSectionsForCM1(userRequest string) map[string][]string {
	return map[string][]string{
		"session_identity":            {"- Session has entered autonomous planning after CM1 question review."},
		"user_request":                {fmt.Sprintf("- Original request: %s", userRequest), "- Restated goal: build a controlled serialized coding system with strong planning and traceability."},
		"locked_decisions":            {"- No clarifying questions required for the mock run.", "- Autonomous mode enabled after CM1 gate."},
		"assumptions":                 {"- The current session focuses on orchestration primitives before provider integrations.", "- Private session artifacts should stay outside the user repo."},
		"product_framing":             {"- This is a desktop-first orchestration engine, not a parallel swarm runner."},
		"competitive":                 {"- Emphasis on auditability, ticket dispatch, and high-planning quality over raw concurrency."},
		"scope_required":              {"- Session state persistence", "- Directive and backlog persistence", "- Serialized dispatch backbone"},
		"scope_best":                  {"- Specialized implementer profiles", "- Rich CM planning artifacts", "- Record implementer completion reports and CM review log"},
		"scope_stretch":               {"- Advanced autonomous replanning", "- Rich analytics and replay UI"},
		"architecture_direction":      {"- Keep runtime state deterministic with JSON artifacts.", "- Keep CM narrative in Markdown."},
		"full_feature_inventory":      {"- Add session workspace inspector", "- Generate first ticket backlog from frozen directive", "- Record implementer completion reports and CM review log"},
		"risks_and_unknowns":          {"- Real provider integration is deferred.", "- User-interactive question gate UI is not yet wired."},
		"backlog_strategy":            {"- Generate atomic tickets after directive freeze."},
		"dispatch_notes_for_final_cm": {"- Prefer state/backend implementers first because the engine core is foundational."},
	}
}

func directiveSectionsForCM2(userRequest string) map[string][]string {
	return map[string][]string{
		"session_identity":            {"- CM2 review completed."},
		"user_request":                {fmt.Sprintf("- Original request: %s", userRequest), "- Restated goal: engineer a serialized orchestration engine that can scale to large backlogs without losing control."},
		"locked_decisions":            {"- Scope remains engine-first.", "- Question gate stayed empty because the request was sufficiently explicit."},
		"assumptions":                 {"- Ticket schema must carry enough detail for focused implementer execution.", "- Review log must be append-only."},
		"product_framing":             {"- The system should feel like an operating discipline, not a chat wrapper."},
		"competitive":                 {"- Differentiation comes from planning quality and deterministic flow rather than speed claims."},
		"scope_required":              {"- Queue mutation helpers", "- Ticket status transitions", "- Implementer completion hand-off"},
		"scope_best":                  {"- Roster specialization", "- Scope classification across backlog items"},
		"scope_stretch":               {"- Automatic ticket splitting heuristics"},
		"architecture_direction":      {"- Engine should own file mutations directly.", "- App shell should stay thin."},
		"full_feature_inventory":      {"- Backlog generation from directive content", "- Implementer reports", "- CM acceptance loop", "- Finalizer summary"},
		"risks_and_unknowns":          {"- Need careful resume semantics once real execution starts."},
		"backlog_strategy":            {"- Keep backlog JSON deterministic and machine-first."},
		"dispatch_notes_for_final_cm": {"- Start with high-leverage foundational tickets, then move to review/reporting."},
	}
}

func directiveSectionsForFinalCM(userRequest string, tickets []TicketFile) map[string][]string {
	return map[string][]string{
		"session_identity":            {"- Final CM has frozen the directive and generated the initial backlog."},
		"user_request":                {fmt.Sprintf("- Original request: %s", userRequest), "- Restated goal: deliver a working orchestration backbone with persistent artifacts and serialized dispatch."},
		"locked_decisions":            {"- Implementation remains serialized.", "- Ticket/report/review persistence is mandatory."},
		"assumptions":                 {"- Mock execution is sufficient to validate the engine skeleton before provider wiring."},
		"product_framing":             {"- CodeDone should behave like a disciplined production pipeline with visible state transitions."},
		"competitive":                 {"- Quality comes from deterministic orchestration and inspectable hand-offs."},
		"scope_required":              {"- Session artifacts", "- Backlog/tickets", "- Reports", "- Review log"},
		"scope_best":                  {"- Specialized implementer profiles", "- Directive traceability back to ticket source refs"},
		"scope_stretch":               {"- Intelligent replanning after blocked tickets"},
		"architecture_direction":      {"- Session workspace remains the orchestration brain.", "- User repo remains the implementation target."},
		"full_feature_inventory":      mockTicketTitles(tickets),
		"risks_and_unknowns":          {"- Real git/provider execution remains future work.", "- Retry and blocked-ticket flows are not implemented yet."},
		"backlog_strategy":            {"- Execute one ticket at a time, with CM review between each attempt.", fmt.Sprintf("- Initial frozen backlog contains %d tickets.", len(tickets))},
		"dispatch_notes_for_final_cm": {"- Assign implementers by profile, not round-robin.", "- Keep one active ticket at a time."},
	}
}

func mockTicketTitles(tickets []TicketFile) []string {
	lines := make([]string, 0, len(tickets))
	for _, ticket := range tickets {
		lines = append(lines, "- "+ticket.ID+": "+ticket.Title)
	}
	return lines
}

func directiveSectionKey(heading string) string {
	switch heading {
	case "## Architecture Direction":
		return "architecture_direction"
	case "## Full Feature Inventory":
		return "full_feature_inventory"
	case "## Risks And Unknowns":
		return "risks_and_unknowns"
	case "## Backlog Strategy":
		return "backlog_strategy"
	case "## Dispatch Notes For Final CM":
		return "dispatch_notes_for_final_cm"
	default:
		return ""
	}
}

func chooseImplementerForTicket(agents []AgentSpec, suggestedProfile string, assignments map[string]int) AgentSpec {
	bestIndex := -1
	bestCount := int(^uint(0) >> 1)
	for i, agent := range agents {
		if !agent.Active {
			continue
		}
		count := assignments[agent.ID]
		if count < bestCount {
			bestCount = count
			bestIndex = i
			continue
		}
		if count == bestCount && bestIndex >= 0 {
			currentMatches := strings.TrimSpace(suggestedProfile) != "" && agents[bestIndex].Profile == suggestedProfile
			candidateMatches := strings.TrimSpace(suggestedProfile) != "" && agent.Profile == suggestedProfile
			if candidateMatches && !currentMatches {
				bestIndex = i
			}
		}
	}
	if bestIndex >= 0 {
		return agents[bestIndex]
	}
	if len(agents) == 0 {
		return AgentSpec{ID: "impl-missing-01", Profile: "unknown", Active: false}
	}
	return agents[0]
}

func loadTicket(path string) (TicketFile, error) {
	var ticket TicketFile
	data, err := os.ReadFile(path)
	if err != nil {
		return TicketFile{}, fmt.Errorf("read ticket %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &ticket); err != nil {
		return TicketFile{}, fmt.Errorf("decode ticket %q: %w", path, err)
	}
	return ticket, nil
}

func appendJSONLine(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal jsonl %q: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open jsonl %q: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append jsonl %q: %w", path, err)
	}
	return nil
}

func removeString(items []string, target string) []string {
	filtered := items[:0]
	for _, item := range items {
		if item != target {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func previousCM(roster RosterFile) string {
	if len(roster.CMAgents) < 2 {
		return ""
	}
	return roster.CMAgents[len(roster.CMAgents)-2].ID
}

func nextReadyTicket(backlog BacklogFile, agents []AgentSpec, workspacePath string) (string, string) {
	for _, ticketID := range backlog.ReadyQueue {
		ticket, err := loadTicket(filepath.Join(workspacePath, "tickets", ticketID+".json"))
		if err != nil {
			continue
		}
		if ticket.Status != TicketTodo {
			continue
		}
		impl := chooseImplementerForTicket(agents, ticket.SuggestedProfile, map[string]int{})
		return ticketID, impl.ID
	}
	return "", ""
}

func runImplementer(ctx context.Context, runner ImplementerRunner, req ImplementerRunRequest) (ImplementerRunResult, error) {
	if runner == nil {
		return ImplementerRunResult{
			Status:            ReportDone,
			Summary:           fmt.Sprintf("No runner configured for %s.", req.Ticket.ID),
			GitDiffRef:        "working-tree",
			SuggestedNextStep: "Configure an implementer runner.",
		}, nil
	}
	return runner.RunImplementer(ctx, req)
}

func maybeWait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// gateWait blocks on the pause gate first, then performs the usual maybeWait.
// Used at every workflow checkpoint so a Pause request takes effect at the
// next safe boundary without killing in-flight provider calls.
func (s *Service) gateWait(ctx context.Context, d time.Duration) error {
	if err := s.gate(ctx); err != nil {
		return err
	}
	return maybeWait(ctx, d)
}

func strPtr(v string) *string {
	return &v
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) <= 7 {
		return sha
	}
	return sha[:7]
}

// countDispatchableTickets returns how many tickets in the snapshot will
// actually be dispatched to an implementer (i.e. ScopeClass != "stretch"
// or any ticket whose status is already past TicketTodo). Stretch tickets
// are auto-deferred at the top of the loop, so they should not inflate the
// "ticket X of N" denominator shown to the user.
func countDispatchableTickets(tickets []TicketFile) int {
	n := 0
	for _, t := range tickets {
		if t.ScopeClass == "stretch" && t.Status == TicketTodo {
			continue
		}
		n++
	}
	return n
}

// ticketDispatchPosition returns the (1-based) position of tickets[i] in the
// dispatch order, plus the total count to display. It walks tickets[0..i]
// to skip auto-deferred stretch tickets so the index matches what the user
// actually sees executing.
//
// If splits later push the slice past originalTotal, the total expands so
// child tickets get a consistent X/N reading instead of "5 of 4".
func ticketDispatchPosition(tickets []TicketFile, i, originalTotal int) (int, int) {
	pos := 0
	for k := 0; k <= i && k < len(tickets); k++ {
		t := tickets[k]
		if t.ScopeClass == "stretch" && t.Status == TicketTodo {
			continue
		}
		pos++
	}
	total := originalTotal
	if i+1 > total {
		total = i + 1
	}
	if pos > total {
		total = pos
	}
	return pos, total
}
