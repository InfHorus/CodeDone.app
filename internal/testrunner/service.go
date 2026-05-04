package testrunner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"codedone/internal/engine"
)

type Service struct {
	timeout time.Duration
}

func NewService() *Service {
	return &Service{timeout: 10 * time.Minute}
}

func (s *Service) Run(ctx context.Context, req engine.TestRunRequest) ([]engine.TestResult, error) {
	commands := detectCommands(req.RepoPath)
	if len(commands) == 0 {
		return []engine.TestResult{{
			Name:    "no-tests-detected",
			Command: "",
			Result:  "not_run",
			Output:  "No supported test command was detected for this repository.",
		}}, nil
	}

	results := make([]engine.TestResult, 0, len(commands))
	for _, cmd := range commands {
		result := engine.TestResult{
			Name:    cmd.name,
			Command: strings.Join(cmd.args, " "),
			Result:  "not_run",
		}
		output, runErr := s.runCommand(ctx, req.RepoPath, cmd.args...)
		result.Output = output
		if runErr != nil {
			result.Result = "fail"
			results = append(results, result)
			return results, nil
		}
		result.Result = "pass"
		results = append(results, result)
	}

	return results, nil
}

type detectedCommand struct {
	name string
	args []string
}

func detectCommands(repoPath string) []detectedCommand {
	commands := []detectedCommand{}
	if fileExists(filepath.Join(repoPath, "go.mod")) {
		commands = append(commands, detectedCommand{
			name: "go-test",
			args: []string{"go", "test", "./..."},
		})
	}
	if fileExists(filepath.Join(repoPath, "package.json")) && pathExists(filepath.Join(repoPath, "node_modules")) {
		commands = append(commands, detectedCommand{
			name: "npm-test",
			args: []string{"npm", "test", "--", "--runInBand"},
		})
	}
	if fileExists(filepath.Join(repoPath, "pyproject.toml")) {
		commands = append(commands, detectedCommand{
			name: "pytest",
			args: []string{"python", "-m", "pytest"},
		})
	}
	return commands
}

func (s *Service) runCommand(ctx context.Context, workDir string, args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing command")
	}
	cmdCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, args[0], args[1:]...)
	cmd.Dir = workDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
	if len(output) > 12000 {
		output = output[:12000]
	}
	if err != nil {
		return output, fmt.Errorf("%s: %w", strings.Join(args, " "), err)
	}
	return output, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
