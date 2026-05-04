package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codedone/internal/engine"
	"codedone/internal/repotools"
)

const (
	previewMaxLines = 30
	previewMaxChars = 240
)

type modelEditResponse struct {
	Status             string               `json:"status"`
	Summary            string               `json:"summary"`
	Operations         []modelFileOperation `json:"operations"`
	TestsRun           []modelTestResult    `json:"tests_run"`
	ImportantDecisions []string             `json:"important_decisions"`
	KnownRisks         []string             `json:"known_risks"`
	RemainingWork      []string             `json:"remaining_work"`
	SuggestedNextStep  string               `json:"suggested_next_step"`
}

type modelFileOperation struct {
	Type       string `json:"type"`
	Path       string `json:"path"`
	Content    string `json:"content,omitempty"`
	OldString  string `json:"oldString,omitempty"`
	NewString  string `json:"newString,omitempty"`
	ReplaceAll bool   `json:"replaceAll,omitempty"`
}

type modelTestResult struct {
	Name   string `json:"name"`
	Result string `json:"result"`
}

func applyFileOperations(repoPath string, operations []modelFileOperation) ([]string, []engine.FileDiffPreview, error) {
	changed := make([]string, 0, len(operations))
	previews := make([]engine.FileDiffPreview, 0, len(operations))
	for _, op := range operations {
		relPath, absPath, err := repotools.ResolveRepoPath(repoPath, op.Path)
		if err != nil {
			return changed, previews, err
		}

		switch strings.TrimSpace(op.Type) {
		case "file_write":
			if _, err := os.Stat(absPath); err == nil {
				return changed, previews, fmt.Errorf("write file %q: file already exists; use file_edit", relPath)
			} else if !os.IsNotExist(err) {
				return changed, previews, fmt.Errorf("write file %q: %w", relPath, err)
			}
			if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
				return changed, previews, fmt.Errorf("create parent dir for %q: %w", relPath, err)
			}
			if err := os.WriteFile(absPath, []byte(op.Content), 0o600); err != nil {
				return changed, previews, fmt.Errorf("write file %q: %w", relPath, err)
			}
			previews = append(previews, buildWritePreview(relPath, absPath, op.Content))
		case "file_delete":
			var beforeContent string
			if data, readErr := os.ReadFile(absPath); readErr == nil {
				beforeContent = string(data)
			}
			if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
				return changed, previews, fmt.Errorf("delete file %q: %w", relPath, err)
			}
			previews = append(previews, buildDeletePreview(relPath, absPath, beforeContent))
		case "file_edit":
			before, after, err := applyExactEdit(absPath, op)
			if err != nil {
				return changed, previews, fmt.Errorf("edit file %q: %w", relPath, err)
			}
			previews = append(previews, buildEditPreview(relPath, absPath, before, after, op))
		default:
			return changed, previews, fmt.Errorf("unsupported file operation type %q", op.Type)
		}

		changed = append(changed, relPath)
	}
	return dedupeStrings(changed), previews, nil
}

func summarizeToolCalls(operations []modelFileOperation, status string) []engine.ToolCall {
	out := make([]engine.ToolCall, 0, len(operations))
	for _, op := range operations {
		name := strings.TrimSpace(op.Type)
		if name == "" {
			continue
		}
		out = append(out, engine.ToolCall{
			Name:   name,
			Target: filepath.ToSlash(filepath.Clean(strings.TrimSpace(op.Path))),
			Status: status,
		})
	}
	return out
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func applyExactEdit(path string, op modelFileOperation) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	contentOld := string(data)
	oldString := strings.TrimSpace(op.OldString)
	if oldString == "" {
		return contentOld, contentOld, fmt.Errorf("file_edit requires non-empty oldString")
	}
	if op.OldString == op.NewString {
		return contentOld, contentOld, fmt.Errorf("no changes")
	}
	ending := repotools.DetectLineEnding(contentOld)
	oldNormalized := repotools.ConvertToLineEnding(repotools.NormalizeLineEndings(op.OldString), ending)
	newNormalized := repotools.ConvertToLineEnding(repotools.NormalizeLineEndings(op.NewString), ending)
	first := strings.Index(contentOld, oldNormalized)
	if first < 0 {
		return contentOld, contentOld, fmt.Errorf("oldString not found")
	}
	last := strings.LastIndex(contentOld, oldNormalized)
	if !op.ReplaceAll && first != last {
		return contentOld, contentOld, fmt.Errorf("multiple matches; provide more context")
	}
	var contentNew string
	if op.ReplaceAll {
		contentNew = strings.ReplaceAll(contentOld, oldNormalized, newNormalized)
	} else {
		contentNew = contentOld[:first] + newNormalized + contentOld[first+len(oldNormalized):]
	}
	if err := os.WriteFile(path, []byte(contentNew), 0o600); err != nil {
		return contentOld, contentOld, err
	}
	return contentOld, contentNew, nil
}

// ── Diff preview builders ─────────────────────────────────────────────────

func splitDiffLines(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(repotools.NormalizeLineEndings(s), "\n")
}

func clipPreviewText(s string) string {
	if len(s) <= previewMaxChars {
		return s
	}
	return s[:previewMaxChars] + "…"
}

func buildWritePreview(relPath, absPath, content string) engine.FileDiffPreview {
	lines := splitDiffLines(content)
	total := len(lines)
	end := total
	if end > previewMaxLines {
		end = previewMaxLines
	}
	out := make([]engine.DiffLine, 0, end)
	for i := 0; i < end; i++ {
		out = append(out, engine.DiffLine{
			Type:   "add",
			NewNum: i + 1,
			Text:   clipPreviewText(lines[i]),
		})
	}
	hidden := 0
	if total > end {
		hidden = total - end
	}
	return engine.FileDiffPreview{
		Op:         "write",
		Path:       relPath,
		AbsPath:    absPath,
		Lines:      out,
		Truncated:  hidden > 0,
		HiddenMore: hidden,
		LineStart:  1,
		LineEnd:    end,
		Adds:       end,
		Removes:    0,
	}
}

func buildDeletePreview(relPath, absPath, beforeContent string) engine.FileDiffPreview {
	lines := splitDiffLines(beforeContent)
	total := len(lines)
	end := total
	if end > previewMaxLines {
		end = previewMaxLines
	}
	out := make([]engine.DiffLine, 0, end)
	for i := 0; i < end; i++ {
		out = append(out, engine.DiffLine{
			Type:   "remove",
			OldNum: i + 1,
			Text:   clipPreviewText(lines[i]),
		})
	}
	hidden := 0
	if total > end {
		hidden = total - end
	}
	return engine.FileDiffPreview{
		Op:         "delete",
		Path:       relPath,
		AbsPath:    absPath,
		Lines:      out,
		Truncated:  hidden > 0,
		HiddenMore: hidden,
		LineStart:  1,
		LineEnd:    end,
		Adds:       0,
		Removes:    end,
	}
}

// buildEditPreview anchors the diff at the change region inside the file
// (using the old-content offset of the matched oldString) so line numbers
// reflect the file, not the trimmed snippet.
func buildEditPreview(relPath, absPath, before, after string, op modelFileOperation) engine.FileDiffPreview {
	ending := repotools.DetectLineEnding(before)
	oldNormalized := repotools.ConvertToLineEnding(repotools.NormalizeLineEndings(op.OldString), ending)
	newNormalized := repotools.ConvertToLineEnding(repotools.NormalizeLineEndings(op.NewString), ending)

	// Anchor: where the first match starts in the original file.
	anchorIdx := strings.Index(before, oldNormalized)
	startLine := 1
	if anchorIdx > 0 {
		startLine = strings.Count(before[:anchorIdx], "\n") + 1
	}

	oldLines := splitDiffLines(strings.TrimRight(oldNormalized, "\r\n"))
	newLines := splitDiffLines(strings.TrimRight(newNormalized, "\r\n"))

	rawDiff := lcsDiffLines(oldLines, newLines, startLine, startLine)

	adds, removes := 0, 0
	for _, l := range rawDiff {
		switch l.Type {
		case "add":
			adds++
		case "remove":
			removes++
		}
	}

	clipped, hidden := capDiffLines(rawDiff, previewMaxLines)
	for i := range clipped {
		clipped[i].Text = clipPreviewText(clipped[i].Text)
	}

	lineStart, lineEnd := startLine, startLine
	if len(rawDiff) > 0 {
		lineStart = firstLineNum(rawDiff)
		lineEnd = lastLineNum(rawDiff)
	}

	return engine.FileDiffPreview{
		Op:         "edit",
		Path:       relPath,
		AbsPath:    absPath,
		Lines:      clipped,
		Truncated:  hidden > 0,
		HiddenMore: hidden,
		LineStart:  lineStart,
		LineEnd:    lineEnd,
		Adds:       adds,
		Removes:    removes,
	}
}

func firstLineNum(lines []engine.DiffLine) int {
	for _, l := range lines {
		if l.OldNum > 0 {
			return l.OldNum
		}
		if l.NewNum > 0 {
			return l.NewNum
		}
	}
	return 0
}

func lastLineNum(lines []engine.DiffLine) int {
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i].OldNum > 0 {
			return lines[i].OldNum
		}
		if lines[i].NewNum > 0 {
			return lines[i].NewNum
		}
	}
	return 0
}

// lcsDiffLines produces a unified-diff-like list of context/add/remove lines
// from old & new line slices. Line numbers are offset by oldStart/newStart so
// callers can anchor the diff inside the source file.
func lcsDiffLines(oldLines, newLines []string, oldStart, newStart int) []engine.DiffLine {
	m, n := len(oldLines), len(newLines)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	out := make([]engine.DiffLine, 0, m+n)
	i, j := 0, 0
	for i < m && j < n {
		if oldLines[i] == newLines[j] {
			out = append(out, engine.DiffLine{Type: "context", OldNum: oldStart + i, NewNum: newStart + j, Text: oldLines[i]})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			out = append(out, engine.DiffLine{Type: "remove", OldNum: oldStart + i, Text: oldLines[i]})
			i++
		} else {
			out = append(out, engine.DiffLine{Type: "add", NewNum: newStart + j, Text: newLines[j]})
			j++
		}
	}
	for ; i < m; i++ {
		out = append(out, engine.DiffLine{Type: "remove", OldNum: oldStart + i, Text: oldLines[i]})
	}
	for ; j < n; j++ {
		out = append(out, engine.DiffLine{Type: "add", NewNum: newStart + j, Text: newLines[j]})
	}
	return out
}

// capDiffLines trims to maxLines, preferring change lines over context.
// Drops trailing context first; if still over, drops leading context; finally
// truncates change lines and reports the hidden remainder count.
func capDiffLines(lines []engine.DiffLine, maxLines int) ([]engine.DiffLine, int) {
	if len(lines) <= maxLines {
		return lines, 0
	}
	firstChange, lastChange := -1, -1
	for idx, l := range lines {
		if l.Type != "context" {
			if firstChange == -1 {
				firstChange = idx
			}
			lastChange = idx
		}
	}
	if firstChange == -1 {
		return lines[:maxLines], len(lines) - maxLines
	}
	const ctxAround = 2
	lo := firstChange - ctxAround
	if lo < 0 {
		lo = 0
	}
	hi := lastChange + ctxAround + 1
	if hi > len(lines) {
		hi = len(lines)
	}
	clipped := lines[lo:hi]
	if len(clipped) <= maxLines {
		return clipped, len(lines) - len(clipped)
	}
	return clipped[:maxLines], len(lines) - maxLines
}
