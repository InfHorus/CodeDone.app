package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

func (s *Service) BuildBacklog(ctx context.Context, workspacePath string, session SessionFile, reviewer AgentSpec, implementers []AgentSpec, cmRunner CMRunner, activity ActivityFunc) (BacklogFile, []TicketFile, error) {
	doc, err := s.LoadDirective(workspacePath)
	if err != nil {
		return BacklogFile{}, nil, err
	}

	plan, err := runBacklogPlan(ctx, cmRunner, BacklogPlanRequest{
		Session:           session,
		CM:                reviewer,
		WorkspacePath:     workspacePath,
		Directive:         doc,
		ImplementerAgents: implementers,
		Activity:          activity,
		Gate:              s.gate,
	})
	if err != nil {
		return BacklogFile{}, nil, err
	}

	blueprints := normalizeBlueprints(plan.Tickets)
	if len(blueprints) == 0 {
		blueprints = fallbackTicketBlueprints(doc)
	}

	tickets, ticketIndex, readyQueue, ticketOrder, err := s.materializeBlueprints(workspacePath, reviewer.ID, blueprints, nil)
	if err != nil {
		return BacklogFile{}, nil, err
	}

	backlog := BacklogFile{
		SessionID:     session.SessionID,
		GeneratedBy:   reviewer.ID,
		GeneratedAt:   s.now(),
		Frozen:        true,
		ScopeSummary:  summarizeScope(tickets),
		TicketOrder:   ticketOrder,
		ReadyQueue:    readyQueue,
		BlockedQueue:  []string{},
		DeferredQueue: []string{},
		DoneQueue:     []string{},
		TicketIndex:   ticketIndex,
	}

	if err := writeJSON(filepath.Join(workspacePath, backlogFileName), backlog); err != nil {
		return BacklogFile{}, nil, err
	}

	return backlog, tickets, nil
}

type ticketSpec struct {
	title             string
	summary           string
	scopeClass        string
	priority          string
	area              string
	profile           string
	risk              string
	complexity        int
	handoff           string
	constraints       []string
	directiveSections []string
}

func collectTicketSpecs(lines []string, scopeClass, priority string) []ticketSpec {
	specs := make([]ticketSpec, 0, len(lines))
	for _, line := range lines {
		title := strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if title == "" {
			continue
		}
		area, profile := inferAreaAndProfile(title)
		specs = append(specs, ticketSpec{
			title:             title,
			summary:           "Implement " + lowerFirst(title) + ".",
			scopeClass:        scopeClass,
			priority:          priority,
			area:              area,
			profile:           profile,
			risk:              "medium",
			complexity:        inferComplexity(title),
			handoff:           "Derived from frozen directive planning.",
			constraints:       []string{"Keep the change scoped to this ticket", "Respect serialized execution"},
			directiveSections: []string{"Directive-derived backlog"},
		})
	}
	return specs
}

func dedupeTicketSpecs(specs []ticketSpec) []ticketSpec {
	seen := map[string]bool{}
	out := make([]ticketSpec, 0, len(specs))
	for _, spec := range specs {
		key := strings.ToLower(strings.TrimSpace(spec.title))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, spec)
	}
	return out
}

func buildAcceptanceCriteria(title string) []string {
	return []string{
		fmt.Sprintf("%s is implemented end to end.", title),
		fmt.Sprintf("%s is reflected in persistent engine artifacts where applicable.", title),
		fmt.Sprintf("%s does not break serialized engine flow.", title),
	}
}

func inferAreaAndProfile(title string) (string, string) {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "ui"), strings.Contains(lower, "frontend"), strings.Contains(lower, "settings"), strings.Contains(lower, "display"):
		return "frontend", "frontend"
	case strings.Contains(lower, "git"), strings.Contains(lower, "branch"), strings.Contains(lower, "diff"):
		return "git", "git"
	case strings.Contains(lower, "report"), strings.Contains(lower, "queue"), strings.Contains(lower, "backlog"), strings.Contains(lower, "state"), strings.Contains(lower, "session"), strings.Contains(lower, "directive"):
		return "engine", "backend"
	case strings.Contains(lower, "test"), strings.Contains(lower, "validation"), strings.Contains(lower, "finalizer"):
		return "validation", "tests"
	default:
		return "engine", "backend"
	}
}

func inferComplexity(title string) int {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "report"), strings.Contains(lower, "queue"), strings.Contains(lower, "state"):
		return 3
	case strings.Contains(lower, "ui"), strings.Contains(lower, "settings"):
		return 2
	default:
		return 2
	}
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func fallbackTicketBlueprints(doc DirectiveDocument) []TicketBlueprint {
	specs := make([]ticketSpec, 0)
	specs = append(specs, collectTicketSpecs(doc.Sections["scope.required"], "required", "high")...)
	specs = append(specs, collectTicketSpecs(doc.Sections["scope.best_in_class"], "best_in_class", "medium")...)
	specs = append(specs, collectTicketSpecs(doc.Sections["scope.stretch"], "stretch", "low")...)
	specs = append(specs, collectTicketSpecs(doc.Sections["full_feature_inventory"], "required", "high")...)

	specs = dedupeTicketSpecs(specs)
	if len(specs) == 0 {
		specs = []ticketSpec{{
			title:       "Establish serialized engine baseline",
			scopeClass:  "required",
			priority:    "high",
			summary:     "Create the baseline orchestration artifact and queue flow.",
			area:        "engine",
			profile:     "backend",
			constraints: []string{"Keep implementation serialized"},
		}}
	}

	out := make([]TicketBlueprint, 0, len(specs))
	for i, spec := range specs {
		ref := fmt.Sprintf("P%d", i+1)
		var depends []string
		if i > 0 {
			depends = []string{fmt.Sprintf("P%d", i)}
		}
		out = append(out, TicketBlueprint{
			Ref:                 ref,
			Title:               spec.title,
			Type:                "feature",
			Area:                spec.area,
			ScopeClass:          spec.scopeClass,
			Priority:            spec.priority,
			Summary:             spec.summary,
			AcceptanceCriteria:  buildAcceptanceCriteria(spec.title),
			Constraints:         spec.constraints,
			SuggestedProfile:    spec.profile,
			EstimatedComplexity: spec.complexity,
			RiskLevel:           spec.risk,
			HandoffNotes:        spec.handoff,
			DependsOnRefs:       depends,
			SourceRefs: SourceRefs{
				DirectiveSections: spec.directiveSections,
				UserAnswers:       []string{},
			},
		})
	}
	return out
}

func normalizeBlueprints(items []TicketBlueprint) []TicketBlueprint {
	out := make([]TicketBlueprint, 0, len(items))
	seen := map[string]bool{}
	for i, item := range items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		key := strings.ToLower(title)
		if seen[key] {
			continue
		}
		seen[key] = true
		if strings.TrimSpace(item.Ref) == "" {
			item.Ref = fmt.Sprintf("P%d", i+1)
		}
		if strings.TrimSpace(item.Type) == "" {
			item.Type = "feature"
		}
		if strings.TrimSpace(item.Area) == "" || strings.TrimSpace(item.SuggestedProfile) == "" {
			area, profile := inferAreaAndProfile(title)
			if strings.TrimSpace(item.Area) == "" {
				item.Area = area
			}
			if strings.TrimSpace(item.SuggestedProfile) == "" {
				item.SuggestedProfile = profile
			}
		}
		if strings.TrimSpace(item.ScopeClass) == "" {
			item.ScopeClass = "required"
		}
		if strings.TrimSpace(item.Priority) == "" {
			item.Priority = "medium"
		}
		if strings.TrimSpace(item.Summary) == "" {
			item.Summary = "Implement " + lowerFirst(title) + "."
		}
		if len(item.AcceptanceCriteria) == 0 {
			item.AcceptanceCriteria = buildAcceptanceCriteria(title)
		}
		if len(item.Constraints) == 0 {
			item.Constraints = []string{"Keep the change scoped to this ticket", "Respect serialized execution"}
		}
		if item.EstimatedComplexity <= 0 {
			item.EstimatedComplexity = inferComplexity(title)
		}
		if strings.TrimSpace(item.RiskLevel) == "" {
			item.RiskLevel = "medium"
		}
		out = append(out, item)
	}
	return out
}

func (s *Service) materializeBlueprints(workspacePath, createdBy string, blueprints []TicketBlueprint, parentTicketID *string) ([]TicketFile, map[string]string, []string, []string, error) {
	tickets := make([]TicketFile, 0, len(blueprints))
	ticketIndex := make(map[string]string, len(blueprints))
	readyQueue := make([]string, 0, len(blueprints))
	ticketOrder := make([]string, 0, len(blueprints))
	refToID := make(map[string]string, len(blueprints))

	for i, spec := range blueprints {
		id := fmt.Sprintf("T-%04d", i+1)
		refToID[spec.Ref] = id
	}
	for i, spec := range blueprints {
		id := refToID[spec.Ref]
		dependsOn := make([]string, 0, len(spec.DependsOnRefs))
		for _, ref := range spec.DependsOnRefs {
			if resolved, ok := refToID[ref]; ok && resolved != id {
				dependsOn = append(dependsOn, resolved)
			}
		}
		ticket := TicketFile{
			ID:                  id,
			Title:               strings.TrimSpace(spec.Title),
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
		tickets = append(tickets, ticket)
		ticketOrder = append(ticketOrder, id)
		readyQueue = append(readyQueue, id)
		ticketIndex[id] = filepath.ToSlash(filepath.Join("tickets", id+".json"))
		if err := writeJSON(filepath.Join(workspacePath, "tickets", id+".json"), ticket); err != nil {
			return nil, nil, nil, nil, err
		}
		if len(ticket.DependsOn) == 0 && i > 0 {
			// preserve serialized bias when the planner leaves dependencies empty
			ticket.DependsOn = []string{tickets[i-1].ID}
			if err := writeJSON(filepath.Join(workspacePath, "tickets", id+".json"), ticket); err != nil {
				return nil, nil, nil, nil, err
			}
			tickets[i] = ticket
		}
	}
	return tickets, ticketIndex, readyQueue, ticketOrder, nil
}

func runBacklogPlan(ctx context.Context, runner CMRunner, req BacklogPlanRequest) (BacklogPlanResult, error) {
	if runner == nil {
		return BacklogPlanResult{}, nil
	}
	return runner.RunBacklogPlan(ctx, req)
}
