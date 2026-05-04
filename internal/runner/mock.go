package runner

import (
	"context"
	"fmt"

	"codedone/internal/engine"
	"codedone/internal/gitops"
)

type MockRunner struct {
	git     *gitops.Service
	workDir string
}

func NewMockRunner(git *gitops.Service, workDir string) *MockRunner {
	return &MockRunner{
		git:     git,
		workDir: workDir,
	}
}

func (r *MockRunner) RunImplementer(ctx context.Context, req engine.ImplementerRunRequest) (engine.ImplementerRunResult, error) {
	select {
	case <-ctx.Done():
		return engine.ImplementerRunResult{}, ctx.Err()
	default:
	}

	result := engine.ImplementerRunResult{
		Status:             engine.ReportDone,
		Summary:            fmt.Sprintf("Mock implementer executed ticket %s without provider-backed code generation yet.", req.Ticket.ID),
		FilesChanged:       []string{},
		Commits:            []string{},
		GitDiffRef:         "working-tree",
		GitDiffStat:        "",
		TestsRun:           []engine.TestResult{},
		ImportantDecisions: []string{"Execution used the provider-agnostic runner contract."},
		KnownRisks: []string{
			"No real provider execution is wired yet.",
		},
		RemainingWork:     []string{},
		SuggestedNextStep: "Replace the mock runner with a provider-backed implementer execution path.",
	}

	if r.git == nil {
		return result, nil
	}

	if diffStat, err := r.git.DiffStat(r.workDir); err == nil {
		result.GitDiffStat = diffStat
	}
	return result, nil
}
