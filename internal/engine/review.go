package engine

import (
	"fmt"
	"path/filepath"
)

func (s *Service) reviseTicket(workspacePath string, backlog BacklogFile, session SessionFile, ticketID, cmID string) (BacklogFile, SessionFile, error) {
	ticket, err := loadTicket(filepath.Join(workspacePath, "tickets", ticketID+".json"))
	if err != nil {
		return backlog, session, err
	}

	ticket.Status = TicketTodo
	ticket.UpdatedAt = s.now()
	ticket.HandoffNotes = ticket.HandoffNotes + " Revised after CM review."
	if err := writeJSON(filepath.Join(workspacePath, "tickets", ticket.ID+".json"), ticket); err != nil {
		return backlog, session, err
	}

	if !containsString(backlog.ReadyQueue, ticketID) {
		backlog.ReadyQueue = append([]string{ticketID}, backlog.ReadyQueue...)
	}

	session.State = StateDispatchReady
	session.ActiveCMID = strPtr(cmID)
	session.ActiveAgentID = nil
	session.ActiveTicketID = nil
	session.BacklogSummary = incrementBacklogState(session.BacklogSummary, TicketInProgress, TicketTodo)
	session.UpdatedAt = s.now()

	if err := writeJSON(filepath.Join(workspacePath, backlogFileName), backlog); err != nil {
		return backlog, session, err
	}
	if err := writeJSON(filepath.Join(workspacePath, sessionFileName), session); err != nil {
		return backlog, session, err
	}

	return backlog, session, nil
}

func (s *Service) deferTicket(workspacePath string, backlog BacklogFile, session SessionFile, ticketID, cmID string) (BacklogFile, SessionFile, error) {
	ticket, err := loadTicket(filepath.Join(workspacePath, "tickets", ticketID+".json"))
	if err != nil {
		return backlog, session, err
	}

	previous := ticket.Status
	ticket.Status = TicketDeferred
	ticket.UpdatedAt = s.now()
	if err := writeJSON(filepath.Join(workspacePath, "tickets", ticket.ID+".json"), ticket); err != nil {
		return backlog, session, err
	}

	backlog.ReadyQueue = removeString(backlog.ReadyQueue, ticketID)
	if !containsString(backlog.DeferredQueue, ticketID) {
		backlog.DeferredQueue = append(backlog.DeferredQueue, ticketID)
	}

	session.State = StateCMReview
	session.ActiveCMID = strPtr(cmID)
	session.ActiveAgentID = nil
	session.ActiveTicketID = nil
	if previous == TicketTodo {
		session.BacklogSummary = incrementBacklogState(session.BacklogSummary, TicketTodo, TicketDeferred)
	}
	session.UpdatedAt = s.now()

	if err := writeJSON(filepath.Join(workspacePath, backlogFileName), backlog); err != nil {
		return backlog, session, err
	}
	if err := writeJSON(filepath.Join(workspacePath, sessionFileName), session); err != nil {
		return backlog, session, err
	}

	return backlog, session, nil
}

func (s *Service) splitTicket(workspacePath string, backlog BacklogFile, session SessionFile, ticketID, cmID string) (BacklogFile, SessionFile, []TicketFile, error) {
	ticket, err := loadTicket(filepath.Join(workspacePath, "tickets", ticketID+".json"))
	if err != nil {
		return backlog, session, nil, err
	}

	ticket.Status = TicketDone
	ticket.UpdatedAt = s.now()
	ticket.HandoffNotes = ticket.HandoffNotes + " Split by CM review into child tickets."
	if err := writeJSON(filepath.Join(workspacePath, "tickets", ticket.ID+".json"), ticket); err != nil {
		return backlog, session, nil, err
	}

	parentID := ticket.ID
	childOneID := nextTicketID(backlog)
	childTwoID := fmt.Sprintf("T-%04d", parseTicketID(childOneID)+1)
	children := []TicketFile{
		{
			ID:                  childOneID,
			Title:               ticket.Title + " - Part 1",
			Type:                ticket.Type,
			Area:                ticket.Area,
			ScopeClass:          ticket.ScopeClass,
			Priority:            ticket.Priority,
			Status:              TicketTodo,
			CreatedBy:           cmID,
			CreatedAt:           s.now(),
			UpdatedAt:           s.now(),
			ParentTicketID:      &parentID,
			DependsOn:           []string{},
			BlockedBy:           []string{},
			SourceRefs:          ticket.SourceRefs,
			Summary:             ticket.Summary + " Focus on the first half of the split.",
			AcceptanceCriteria:  firstHalf(ticket.AcceptanceCriteria),
			Constraints:         ticket.Constraints,
			SuggestedProfile:    ticket.SuggestedProfile,
			EstimatedComplexity: max(ticket.EstimatedComplexity-1, 1),
			RiskLevel:           ticket.RiskLevel,
			HandoffNotes:        "Child ticket created by split decision.",
		},
		{
			ID:                  childTwoID,
			Title:               ticket.Title + " - Part 2",
			Type:                ticket.Type,
			Area:                ticket.Area,
			ScopeClass:          ticket.ScopeClass,
			Priority:            ticket.Priority,
			Status:              TicketTodo,
			CreatedBy:           cmID,
			CreatedAt:           s.now(),
			UpdatedAt:           s.now(),
			ParentTicketID:      &parentID,
			DependsOn:           []string{childOneID},
			BlockedBy:           []string{},
			SourceRefs:          ticket.SourceRefs,
			Summary:             ticket.Summary + " Focus on the second half of the split.",
			AcceptanceCriteria:  secondHalf(ticket.AcceptanceCriteria),
			Constraints:         ticket.Constraints,
			SuggestedProfile:    ticket.SuggestedProfile,
			EstimatedComplexity: max(ticket.EstimatedComplexity-1, 1),
			RiskLevel:           ticket.RiskLevel,
			HandoffNotes:        "Child ticket created by split decision.",
		},
	}

	for _, child := range children {
		if err := writeJSON(filepath.Join(workspacePath, "tickets", child.ID+".json"), child); err != nil {
			return backlog, session, nil, err
		}
		backlog.TicketOrder = append(backlog.TicketOrder, child.ID)
		backlog.ReadyQueue = append(backlog.ReadyQueue, child.ID)
		backlog.TicketIndex[child.ID] = filepath.ToSlash(filepath.Join("tickets", child.ID+".json"))
	}
	backlog.DoneQueue = append(backlog.DoneQueue, ticketID)

	session.State = StateDispatchReady
	session.ActiveCMID = strPtr(cmID)
	session.ActiveAgentID = nil
	session.ActiveTicketID = nil
	session.BacklogSummary.Done++
	session.BacklogSummary.Total += len(children)
	session.BacklogSummary.Todo += len(children)
	if session.BacklogSummary.InProgress > 0 {
		session.BacklogSummary.InProgress--
	}
	session.UpdatedAt = s.now()

	if err := writeJSON(filepath.Join(workspacePath, backlogFileName), backlog); err != nil {
		return backlog, session, nil, err
	}
	if err := writeJSON(filepath.Join(workspacePath, sessionFileName), session); err != nil {
		return backlog, session, nil, err
	}

	return backlog, session, children, nil
}

func (s *Service) splitTicketWithBlueprints(workspacePath string, backlog BacklogFile, session SessionFile, ticketID, cmID string, blueprints []TicketBlueprint) (BacklogFile, SessionFile, []TicketFile, error) {
	if len(blueprints) == 0 {
		return s.splitTicket(workspacePath, backlog, session, ticketID, cmID)
	}

	ticket, err := loadTicket(filepath.Join(workspacePath, "tickets", ticketID+".json"))
	if err != nil {
		return backlog, session, nil, err
	}

	ticket.Status = TicketDone
	ticket.UpdatedAt = s.now()
	ticket.HandoffNotes = ticket.HandoffNotes + " Split by CM review into provider-defined child tickets."
	if err := writeJSON(filepath.Join(workspacePath, "tickets", ticket.ID+".json"), ticket); err != nil {
		return backlog, session, nil, err
	}

	parentID := ticket.ID
	children, err := s.materializeChildBlueprints(workspacePath, cmID, blueprints, &parentID, backlog)
	if err != nil {
		return backlog, session, nil, err
	}
	for _, child := range children {
		backlog.TicketOrder = append(backlog.TicketOrder, child.ID)
		backlog.ReadyQueue = append(backlog.ReadyQueue, child.ID)
		backlog.TicketIndex[child.ID] = filepath.ToSlash(filepath.Join("tickets", child.ID+".json"))
	}
	backlog.DoneQueue = append(backlog.DoneQueue, ticketID)

	session.State = StateDispatchReady
	session.ActiveCMID = strPtr(cmID)
	session.ActiveAgentID = nil
	session.ActiveTicketID = nil
	session.BacklogSummary.Done++
	session.BacklogSummary.Total += len(children)
	session.BacklogSummary.Todo += len(children)
	if session.BacklogSummary.InProgress > 0 {
		session.BacklogSummary.InProgress--
	}
	session.UpdatedAt = s.now()

	if err := writeJSON(filepath.Join(workspacePath, backlogFileName), backlog); err != nil {
		return backlog, session, nil, err
	}
	if err := writeJSON(filepath.Join(workspacePath, sessionFileName), session); err != nil {
		return backlog, session, nil, err
	}

	return backlog, session, children, nil
}

func (s *Service) blockTicket(workspacePath string, backlog BacklogFile, session SessionFile, ticketID, cmID string) (BacklogFile, SessionFile, error) {
	ticket, err := loadTicket(filepath.Join(workspacePath, "tickets", ticketID+".json"))
	if err != nil {
		return backlog, session, err
	}

	previous := ticket.Status
	ticket.Status = TicketBlocked
	ticket.UpdatedAt = s.now()
	if err := writeJSON(filepath.Join(workspacePath, "tickets", ticket.ID+".json"), ticket); err != nil {
		return backlog, session, err
	}

	backlog.ReadyQueue = removeString(backlog.ReadyQueue, ticketID)
	if !containsString(backlog.BlockedQueue, ticketID) {
		backlog.BlockedQueue = append(backlog.BlockedQueue, ticketID)
	}
	session.State = StateCMReview
	session.ActiveCMID = strPtr(cmID)
	session.ActiveAgentID = nil
	session.ActiveTicketID = nil
	switch previous {
	case TicketTodo:
		session.BacklogSummary.Todo--
	case TicketInProgress:
		if session.BacklogSummary.InProgress > 0 {
			session.BacklogSummary.InProgress--
		}
	}
	session.BacklogSummary.Blocked++
	session.UpdatedAt = s.now()

	if err := writeJSON(filepath.Join(workspacePath, backlogFileName), backlog); err != nil {
		return backlog, session, err
	}
	if err := writeJSON(filepath.Join(workspacePath, sessionFileName), session); err != nil {
		return backlog, session, err
	}
	return backlog, session, nil
}

func nextTicketID(backlog BacklogFile) string {
	maxID := 0
	for _, id := range backlog.TicketOrder {
		if n := parseTicketID(id); n > maxID {
			maxID = n
		}
	}
	return fmt.Sprintf("T-%04d", maxID+1)
}

func (s *Service) materializeChildBlueprints(workspacePath, createdBy string, blueprints []TicketBlueprint, parentTicketID *string, backlog BacklogFile) ([]TicketFile, error) {
	normalized := normalizeBlueprints(blueprints)
	if len(normalized) == 0 {
		return []TicketFile{}, nil
	}
	nextID := parseTicketID(nextTicketID(backlog))
	refToID := map[string]string{}
	for i, blueprint := range normalized {
		refToID[blueprint.Ref] = fmt.Sprintf("T-%04d", nextID+i)
	}
	children := make([]TicketFile, 0, len(normalized))
	for i, spec := range normalized {
		id := refToID[spec.Ref]
		dependsOn := make([]string, 0, len(spec.DependsOnRefs))
		for _, ref := range spec.DependsOnRefs {
			if resolved, ok := refToID[ref]; ok && resolved != id {
				dependsOn = append(dependsOn, resolved)
			}
		}
		if len(dependsOn) == 0 && i > 0 {
			dependsOn = []string{children[i-1].ID}
		}
		child := TicketFile{
			ID:                  id,
			Title:               spec.Title,
			Type:                spec.Type,
			Area:                spec.Area,
			ScopeClass:          spec.ScopeClass,
			Priority:            spec.Priority,
			Status:              TicketTodo,
			CreatedBy:           createdBy,
			CreatedAt:           s.now(),
			UpdatedAt:           s.now(),
			ParentTicketID:      parentTicketID,
			DependsOn:           dependsOn,
			BlockedBy:           []string{},
			SourceRefs:          spec.SourceRefs,
			Summary:             spec.Summary,
			AcceptanceCriteria:  ensureBulletLines(spec.AcceptanceCriteria),
			Constraints:         ensureBulletLines(spec.Constraints),
			SuggestedProfile:    spec.SuggestedProfile,
			EstimatedComplexity: spec.EstimatedComplexity,
			RiskLevel:           spec.RiskLevel,
			HandoffNotes:        spec.HandoffNotes,
		}
		children = append(children, child)
		if err := writeJSON(filepath.Join(workspacePath, "tickets", child.ID+".json"), child); err != nil {
			return nil, err
		}
	}
	return children, nil
}

func parseTicketID(id string) int {
	var n int
	fmt.Sscanf(id, "T-%d", &n)
	return n
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func firstHalf(items []string) []string {
	if len(items) <= 1 {
		return items
	}
	return append([]string{}, items[:1]...)
}

func secondHalf(items []string) []string {
	if len(items) <= 1 {
		return items
	}
	return append([]string{}, items[1:]...)
}
