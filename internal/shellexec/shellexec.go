// Package shellexec provides the cross-platform shell tool used by both the
// implementer and CM agents. It runs arbitrary commands through the platform
// native shell (cmd /C on Windows, sh -c everywhere else) with a bounded
// timeout and output cap so LLM context is not flooded.
package shellexec

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"codedone/internal/engine"
)

// SystemInfo returns OS, shell, and current time info for the get_system_info tool.
func SystemInfo() map[string]any {
	now := time.Now()
	shell := "sh"
	if runtime.GOOS == "windows" {
		shell = "cmd"
	}
	return map[string]any{
		"ok":        true,
		"os":        runtime.GOOS,
		"arch":      runtime.GOARCH,
		"shell":     shell,
		"date":      now.Format("2006-01-02"),
		"time":      now.Format("15:04:05"),
		"timestamp": now.Format(time.RFC3339),
		"timezone":  now.Location().String(),
	}
}

const (
	defaultTimeoutMs = 30_000
	maxTimeoutMs     = 120_000
	maxOutputBytes   = 8 * 1024
)

type ShellArgs struct {
	Command   string `json:"command"`
	Cwd       string `json:"cwd,omitempty"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

// ExecuteShell runs args.Command through the platform shell inside args.Cwd
// (defaults to repoPath). Output is capped at 8 KB; exit-code and timeout
// information are appended as plain-text annotations so the model can react.
func ExecuteShell(repoPath string, args ShellArgs) (string, engine.ToolCall, error) {
	record := engine.ToolCall{Name: "shell", Target: args.Command}

	command := strings.TrimSpace(args.Command)
	if command == "" {
		record.Status = "failed"
		return `{"ok":false,"error":"command is required"}`, record, nil
	}

	timeoutMs := args.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultTimeoutMs
	}
	if timeoutMs > maxTimeoutMs {
		timeoutMs = maxTimeoutMs
	}

	cwd := strings.TrimSpace(args.Cwd)
	if cwd == "" {
		cwd = repoPath
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Dir = cwd

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	runErr := cmd.Run()
	output := buf.String()

	if ctx.Err() == context.DeadlineExceeded {
		if len(output) > maxOutputBytes {
			output = output[:maxOutputBytes] + fmt.Sprintf("\n[shell: output truncated at %d bytes]", maxOutputBytes)
		}
		output += fmt.Sprintf("\n[shell: timed out after %dms — increase timeout_ms if the command needs more time]", timeoutMs)
		record.Status = "failed"
		return output, record, nil
	}

	truncated := len(output) > maxOutputBytes
	if truncated {
		output = output[:maxOutputBytes] + fmt.Sprintf("\n[shell: output truncated at %d bytes]", maxOutputBytes)
	}

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	if exitCode != 0 {
		output += fmt.Sprintf("\n[exit code %d]", exitCode)
		record.Status = "failed"
	} else {
		record.Status = "done"
	}

	return output, record, nil
}
