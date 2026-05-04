package gitops

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Service struct {
	gitPath string
	timeout time.Duration
}

type RepoSnapshot struct {
	Path          string   `json:"path"`
	Root          string   `json:"root"`
	IsRepo        bool     `json:"is_repo"`
	HeadBranch    string   `json:"head_branch"`
	HeadCommit    string   `json:"head_commit"`
	Dirty         bool     `json:"dirty"`
	StatusLines   []string `json:"status_lines"`
	SessionBranch string   `json:"session_branch,omitempty"`
}

func NewService(gitPath string) *Service {
	if strings.TrimSpace(gitPath) == "" {
		gitPath = "git"
	}
	return &Service{
		gitPath: gitPath,
		timeout: 15 * time.Second,
	}
}

func (s *Service) InspectRepo(workDir string) (RepoSnapshot, error) {
	root, err := s.runTrimmed(workDir, "rev-parse", "--show-toplevel")
	if err != nil {
		if isNotRepoError(err) {
			return RepoSnapshot{
				Path:   workDir,
				Root:   "",
				IsRepo: false,
			}, nil
		}
		return RepoSnapshot{}, err
	}

	branch, err := s.runTrimmed(workDir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		branch = ""
	}
	commit, err := s.runTrimmed(workDir, "rev-parse", "HEAD")
	if err != nil {
		if isUnbornHeadError(err) {
			commit = ""
		} else {
			return RepoSnapshot{}, err
		}
	}
	statusRaw, err := s.runTrimmed(workDir, "status", "--short")
	if err != nil {
		return RepoSnapshot{}, err
	}

	statusLines := []string{}
	if statusRaw != "" {
		statusLines = strings.Split(statusRaw, "\n")
	}

	return RepoSnapshot{
		Path:        workDir,
		Root:        filepath.Clean(root),
		IsRepo:      true,
		HeadBranch:  branch,
		HeadCommit:  commit,
		Dirty:       len(statusLines) > 0,
		StatusLines: statusLines,
	}, nil
}

func (s *Service) EnsureBranch(workDir, branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", fmt.Errorf("branch name is empty")
	}

	_, err := s.runTrimmed(workDir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	exists := err == nil
	if exists {
		if _, err := s.runTrimmed(workDir, "checkout", branch); err != nil {
			return "", err
		}
		return branch, nil
	}

	if _, err := s.runTrimmed(workDir, "checkout", "-b", branch); err != nil {
		return "", err
	}
	return branch, nil
}

func (s *Service) DiffStat(workDir string) (string, error) {
	return s.runTrimmed(workDir, "diff", "--stat")
}

func (s *Service) Diff(workDir string) (string, error) {
	return s.runTrimmed(workDir, "diff")
}

func (s *Service) DiffNames(workDir string) ([]string, error) {
	out, err := s.runTrimmed(workDir, "diff", "--name-only")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(out, "\n"), nil
}

func (s *Service) ListFiles(workDir string) ([]string, error) {
	out, err := s.runTrimmed(workDir, "ls-files", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(out, "\n"), nil
}

// StageAll stages every working-tree change (modified, untracked, deleted)
// before a session-end commit.
func (s *Service) StageAll(workDir string) error {
	_, err := s.runTrimmed(workDir, "add", "-A")
	return err
}

// Commit creates a commit with the supplied message. Returns the new HEAD SHA.
// If there is nothing staged, returns ("", nil) so callers can no-op cleanly.
func (s *Service) Commit(workDir, message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "CodeDone session changes"
	}
	if err := s.StageAll(workDir); err != nil {
		return "", err
	}
	if status, err := s.runTrimmed(workDir, "status", "--porcelain"); err == nil && status == "" {
		return "", nil
	}
	if _, err := s.runTrimmed(workDir, "commit", "-m", message); err != nil {
		return "", err
	}
	return s.runTrimmed(workDir, "rev-parse", "HEAD")
}

func (s *Service) HeadCommit(workDir string) (string, error) {
	out, err := s.runTrimmed(workDir, "rev-parse", "HEAD")
	if err != nil && isUnbornHeadError(err) {
		return "", nil
	}
	return out, err
}

func (s *Service) runTrimmed(workDir string, args ...string) (string, error) {
	out, err := s.run(workDir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (s *Service) run(workDir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, s.gitPath, args...)
	cmd.Dir = workDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}

	return stdout.String(), nil
}

func isNotRepoError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "not a git repository")
}

func isUnbornHeadError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "unknown revision or path not in the working tree") ||
		strings.Contains(msg, "ambiguous argument 'HEAD'") ||
		strings.Contains(msg, "bad revision 'HEAD'") ||
		strings.Contains(msg, "Needed a single revision")
}
