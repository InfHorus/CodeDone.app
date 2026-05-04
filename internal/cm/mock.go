package cm

import (
	"context"
	"fmt"

	"codedone/internal/engine"
)

type MockRunner struct{}

func NewMockRunner() *MockRunner {
	return &MockRunner{}
}

func (r *MockRunner) RunPlan(ctx context.Context, req engine.CMPlanRequest) (engine.CMPlanResult, error) {
	select {
	case <-ctx.Done():
		return engine.CMPlanResult{}, ctx.Err()
	default:
	}
	return engine.CMPlanResult{
		Summary: "Mock plan-mode CM answered without dispatching implementers or creating tickets.",
		Answer:  fmt.Sprintf("%s mock plan response:\n\n%s", req.CM.ID, req.UserMessage),
	}, nil
}

func (r *MockRunner) RunQuestionGate(ctx context.Context, req engine.CMQuestionGateRequest) (engine.CMQuestionGateResult, error) {
	select {
	case <-ctx.Done():
		return engine.CMQuestionGateResult{}, ctx.Err()
	default:
	}
	return engine.CMQuestionGateResult{
		Needed:  false,
		Summary: fmt.Sprintf("%s mock question gate found the request clear enough to proceed.", req.CM.ID),
		LockedDecisions: []string{
			"- Mock CM runner proceeded without user questions.",
		},
		Assumptions: []string{
			"- Replace the mock CM runner with a provider-backed planning path.",
		},
	}, nil
}

func (r *MockRunner) RunDirectivePass(ctx context.Context, req engine.CMDirectiveRequest) (engine.CMDirectiveResult, error) {
	select {
	case <-ctx.Done():
		return engine.CMDirectiveResult{}, ctx.Err()
	default:
	}
	return engine.CMDirectiveResult{
		Summary: fmt.Sprintf("%s mock directive pass completed for version %d.", req.CM.ID, req.Version),
		Sections: map[string][]string{
			"session_identity":            {"- Mock CM directive pass completed."},
			"user_request":                {"- Original request preserved.", "- Restated goal: build the serialized orchestration engine cleanly."},
			"locked_decisions":            {"- Mock runner kept the session autonomous."},
			"assumptions":                 {"- Replace mock CM planning with a real provider later."},
			"product_framing":             {"- Prioritize deterministic orchestration over swarm behavior."},
			"competitive":                 {"- Differentiate on planning rigor and inspectability."},
			"scope_required":              {"- Question gate", "- Directive generation", "- Serialized ticket dispatch"},
			"scope_best":                  {"- Specialized implementer profiles", "- Strong review checkpoints"},
			"scope_stretch":               {"- Replay and analytics"},
			"architecture_direction":      {"- Keep artifacts deterministic.", "- Keep planning narrative readable."},
			"full_feature_inventory":      {"- Generate directive", "- Generate backlog", "- Dispatch tickets"},
			"risks_and_unknowns":          {"- Mock runner output is intentionally shallow."},
			"backlog_strategy":            {"- Freeze a queue after directive finalization."},
			"dispatch_notes_for_final_cm": {"- Assign one ticket at a time."},
		},
	}, nil
}

func (r *MockRunner) RunBacklogPlan(ctx context.Context, req engine.BacklogPlanRequest) (engine.BacklogPlanResult, error) {
	select {
	case <-ctx.Done():
		return engine.BacklogPlanResult{}, ctx.Err()
	default:
	}
	return engine.BacklogPlanResult{
		Summary: "Mock CM backlog planner generated a minimal serialized backlog.",
		Tickets: []engine.TicketBlueprint{
			{
				Ref:                 "P1",
				Title:               "Establish serialized engine baseline",
				Type:                "feature",
				Area:                "engine",
				ScopeClass:          "required",
				Priority:            "high",
				Summary:             "Create the baseline orchestration artifact and queue flow.",
				AcceptanceCriteria:  []string{"- Engine baseline is created end to end."},
				Constraints:         []string{"- Keep implementation serialized."},
				SuggestedProfile:    "backend",
				EstimatedComplexity: 2,
				RiskLevel:           "medium",
				HandoffNotes:        "Mock backlog item.",
			},
		},
	}, nil
}

func (r *MockRunner) RunReview(ctx context.Context, req engine.CMReviewRequest) (engine.CMReviewResult, error) {
	select {
	case <-ctx.Done():
		return engine.CMReviewResult{}, ctx.Err()
	default:
	}
	return engine.CMReviewResult{
		Summary:  fmt.Sprintf("%s mock review accepted %s.", req.CM.ID, req.Ticket.ID),
		Decision: engine.DecisionAccept,
		Reason:   "Mock review path accepted the attempt.",
	}, nil
}

func (r *MockRunner) RunFinalizer(ctx context.Context, req engine.FinalizerRequest) (engine.FinalizerResult, error) {
	select {
	case <-ctx.Done():
		return engine.FinalizerResult{}, ctx.Err()
	default:
	}
	return engine.FinalizerResult{
		Summary:    "Mock finalizer wrote a summary report.",
		ReportBody: "Mock finalizer report.",
	}, nil
}
