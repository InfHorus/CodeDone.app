// Package repotools exposes the read-only repo-browsing tools (list, file_read,
// glob, grep) used by both the implementer (file mutations) and the CM
// (planning and review). Mutations live in the runner package because they are
// implementer-only.
package repotools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"codedone/internal/engine"
)

const (
	defaultReadLimit = 2000
	maxReadLimit     = 2000
	maxReadLineChars = 2000
	maxReadBytes     = 50 * 1024
	maxListResults   = 100
	maxSearchResults = 100
)

var defaultIgnorePatterns = []string{
	"node_modules/",
	"__pycache__/",
	".git/",
	"dist/",
	"build/",
	"target/",
	"vendor/",
	"bin/",
	"obj/",
	".idea/",
	".vscode/",
	".venv/",
	"venv/",
	"env/",
	"coverage/",
	"tmp/",
	"cache/",
	"logs/",
}

type ListArgs struct {
	Path   string   `json:"path"`
	Ignore []string `json:"ignore"`
}

type GlobArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

type GrepArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Include string `json:"include"`
}

type FileReadArgs struct {
	FilePath string `json:"filePath"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

type fileMatch struct {
	RelPath string
	ModTime time.Time
}

type grepMatch struct {
	RelPath string
	Line    int
	Text    string
	ModTime time.Time
}

func ExecuteList(repoPath string, args ListArgs) (string, engine.ToolCall, error) {
	relPath, absPath, err := ResolveBrowsePath(repoPath, args.Path)
	call := engine.ToolCall{Name: "list", Target: relPath}
	if err != nil {
		call.Status = "failed"
		return "", call, err
	}
	files, err := collectPaths(absPath, buildIgnoreMatcher(args.Ignore), func(string) bool { return true }, maxListResults)
	if err != nil {
		call.Status = "failed"
		return "", call, err
	}
	call.Status = "done"
	return renderListResult(relPath, files), call, nil
}

func ExecuteGlob(repoPath string, args GlobArgs) (string, engine.ToolCall, error) {
	relPath, absPath, err := ResolveBrowsePath(repoPath, args.Path)
	call := engine.ToolCall{Name: "glob", Target: relPath}
	if err != nil {
		call.Status = "failed"
		return "", call, err
	}
	pattern := strings.TrimSpace(args.Pattern)
	if pattern == "" {
		call.Status = "failed"
		return "", call, fmt.Errorf("glob pattern is empty")
	}
	files, err := collectPaths(absPath, buildIgnoreMatcher(nil), func(rel string) bool {
		return MatchGlobPattern(pattern, rel)
	}, maxSearchResults)
	if err != nil {
		call.Status = "failed"
		return "", call, err
	}
	call.Status = "done"
	return renderSearchPaths(relPath, "glob", pattern, files), call, nil
}

func ExecuteGrep(repoPath string, args GrepArgs) (string, engine.ToolCall, error) {
	relPath, absPath, err := ResolveBrowsePath(repoPath, args.Path)
	call := engine.ToolCall{Name: "grep", Target: relPath}
	if err != nil {
		call.Status = "failed"
		return "", call, err
	}
	pattern := strings.TrimSpace(args.Pattern)
	if pattern == "" {
		call.Status = "failed"
		return "", call, fmt.Errorf("grep pattern is empty")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		call.Status = "failed"
		return "", call, fmt.Errorf("compile grep pattern: %w", err)
	}
	matches, err := collectGrepMatches(absPath, buildIgnoreMatcher(nil), re, strings.TrimSpace(args.Include))
	if err != nil {
		call.Status = "failed"
		return "", call, err
	}
	call.Status = "done"
	return renderGrepResult(relPath, pattern, matches), call, nil
}

func ExecuteFileRead(repoPath string, args FileReadArgs) (string, engine.ToolCall, error) {
	relPath, absPath, err := ResolveBrowsePath(repoPath, args.FilePath)
	call := engine.ToolCall{Name: "file_read", Target: relPath}
	if err != nil {
		call.Status = "failed"
		return "", call, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		call.Status = "failed"
		return "", call, err
	}
	offset := args.Offset
	if offset <= 0 {
		offset = 1
	}
	limit := args.Limit
	if limit <= 0 {
		limit = defaultReadLimit
	}
	if limit > maxReadLimit {
		limit = maxReadLimit
	}
	var output string
	if info.IsDir() {
		output, err = readDirectory(absPath, relPath, offset, limit)
	} else {
		output, err = readFileLines(absPath, relPath, offset, limit)
	}
	if err != nil {
		call.Status = "failed"
		return "", call, err
	}
	call.Status = "done"
	return output, call, nil
}

func ResolveRepoPath(repoPath, relPath string) (string, string, error) {
	trimmed := strings.TrimSpace(relPath)
	if trimmed == "" {
		return "", "", fmt.Errorf("operation path is empty")
	}
	cleanRel := filepath.Clean(trimmed)
	if cleanRel == "." {
		return "", "", fmt.Errorf("operation path resolved to repo root")
	}
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve repo path: %w", err)
	}
	absTarget := filepath.Join(absRepo, cleanRel)
	absTarget, err = filepath.Abs(absTarget)
	if err != nil {
		return "", "", fmt.Errorf("resolve target path: %w", err)
	}
	prefix := absRepo + string(os.PathSeparator)
	if absTarget != absRepo && !strings.HasPrefix(absTarget, prefix) {
		return "", "", fmt.Errorf("path %q escapes repo root", relPath)
	}
	return filepath.ToSlash(cleanRel), absTarget, nil
}

func ResolveBrowsePath(repoPath, relPath string) (string, string, error) {
	trimmed := strings.TrimSpace(relPath)
	if trimmed == "" || trimmed == "." {
		absRepo, err := filepath.Abs(repoPath)
		if err != nil {
			return "", "", fmt.Errorf("resolve repo path: %w", err)
		}
		return ".", absRepo, nil
	}
	return ResolveRepoPath(repoPath, relPath)
}

func NormalizeLineEndings(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	return input
}

func DetectLineEnding(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func ConvertToLineEnding(input, ending string) string {
	if ending == "\n" {
		return input
	}
	return strings.ReplaceAll(input, "\n", ending)
}

func MarshalToolResult(payload any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return `{"ok":false,"error":"tool result marshal failed"}`
	}
	return string(data)
}

func MatchGlobPattern(pattern, rel string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	rel = filepath.ToSlash(rel)
	if pattern == "" {
		return false
	}
	re, err := regexp.Compile(globToRegex(pattern))
	if err != nil {
		ok, fallbackErr := path.Match(pattern, rel)
		return fallbackErr == nil && ok
	}
	return re.MatchString(rel)
}

func readDirectory(absPath, relPath string, offset, limit int) (string, error) {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return renderNumberedLines(relPath, names, offset, limit), nil
}

func readFileLines(absPath, relPath string, offset, limit int) (string, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", fmt.Errorf("binary files are not supported by file_read")
	}
	content := NormalizeLineEndings(string(data))
	lines := strings.Split(content, "\n")
	return renderNumberedLines(relPath, lines, offset, limit), nil
}

func renderNumberedLines(relPath string, lines []string, offset, limit int) string {
	total := len(lines)
	if total == 0 {
		total = 1
		lines = []string{""}
	}
	start := offset
	if start < 1 {
		start = 1
	}
	if start > total {
		start = total
	}
	end := start + limit - 1
	if end > total {
		end = total
	}
	var b strings.Builder
	b.WriteString(relPath)
	b.WriteString("\n")
	b.WriteString("file\n\n")
	currentBytes := b.Len()
	lastWritten := start - 1
	for i := start; i <= end; i++ {
		line := lines[i-1]
		if len(line) > maxReadLineChars {
			line = line[:maxReadLineChars]
		}
		rendered := strconv.Itoa(i) + ": " + line + "\n"
		if currentBytes+len(rendered) > maxReadBytes {
			break
		}
		b.WriteString(rendered)
		currentBytes += len(rendered)
		lastWritten = i
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("(Showing lines %d-%d of %d", start, lastWritten, total))
	if lastWritten < total {
		b.WriteString(fmt.Sprintf(". Use offset=%d to continue.", lastWritten+1))
	}
	b.WriteString(")")
	return b.String()
}

func buildIgnoreMatcher(extra []string) func(string, bool) bool {
	combined := append([]string{}, defaultIgnorePatterns...)
	combined = append(combined, extra...)
	return func(rel string, isDir bool) bool {
		normalized := filepath.ToSlash(rel)
		if normalized == "." || normalized == "" {
			return false
		}
		if isDir && !strings.HasSuffix(normalized, "/") {
			normalized += "/"
		}
		for _, pattern := range combined {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				continue
			}
			if strings.HasSuffix(pattern, "/") {
				if strings.Contains(normalized, pattern) || strings.HasPrefix(normalized, pattern) {
					return true
				}
				continue
			}
			if MatchGlobPattern(pattern, normalized) {
				return true
			}
		}
		return false
	}
}

func collectPaths(basePath string, ignore func(string, bool) bool, match func(string) bool, limit int) ([]fileMatch, error) {
	results := []fileMatch{}
	err := filepath.WalkDir(basePath, func(current string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(basePath, current)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if ignore(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !match(rel) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		results = append(results, fileMatch{RelPath: rel, ModTime: info.ModTime()})
		if len(results) >= limit {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil && err != fs.SkipAll {
		return nil, err
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].ModTime.Equal(results[j].ModTime) {
			return results[i].RelPath < results[j].RelPath
		}
		return results[i].ModTime.After(results[j].ModTime)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func collectGrepMatches(basePath string, ignore func(string, bool) bool, re *regexp.Regexp, include string) ([]grepMatch, error) {
	results := []grepMatch{}
	err := filepath.WalkDir(basePath, func(current string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(basePath, current)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if ignore(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if include != "" && !MatchGlobPattern(include, rel) {
			return nil
		}
		data, readErr := os.ReadFile(current)
		if readErr != nil || bytes.IndexByte(data, 0) >= 0 {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		lines := strings.Split(NormalizeLineEndings(string(data)), "\n")
		for i, line := range lines {
			if !re.MatchString(line) {
				continue
			}
			text := line
			if len(text) > maxReadLineChars {
				text = text[:maxReadLineChars]
			}
			results = append(results, grepMatch{
				RelPath: rel,
				Line:    i + 1,
				Text:    text,
				ModTime: info.ModTime(),
			})
			if len(results) >= maxSearchResults {
				return fs.SkipAll
			}
		}
		return nil
	})
	if err != nil && err != fs.SkipAll {
		return nil, err
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].ModTime.Equal(results[j].ModTime) {
			if results[i].RelPath == results[j].RelPath {
				return results[i].Line < results[j].Line
			}
			return results[i].RelPath < results[j].RelPath
		}
		return results[i].ModTime.After(results[j].ModTime)
	})
	if len(results) > maxSearchResults {
		results = results[:maxSearchResults]
	}
	return results, nil
}

func renderListResult(relPath string, files []fileMatch) string {
	if len(files) == 0 {
		if relPath == "." || relPath == "" {
			return relPath + "\ntree\n\n(repository is empty — no source files exist yet)\n"
		}
		return relPath + "\ntree\n\n(no files found in this directory)\n"
	}
	lines := make([]string, 0, len(files))
	for _, item := range files {
		lines = append(lines, item.RelPath)
	}
	return relPath + "\ntree\n\n" + strings.Join(lines, "\n")
}

func renderSearchPaths(relPath, toolName, pattern string, files []fileMatch) string {
	header := fmt.Sprintf("%s results for %q under %s", toolName, pattern, relPath)
	if len(files) == 0 {
		return header + "\n\n(no matches found)\n"
	}
	lines := make([]string, 0, len(files))
	for _, item := range files {
		lines = append(lines, item.RelPath)
	}
	return header + "\n\n" + strings.Join(lines, "\n")
}

func renderGrepResult(relPath, pattern string, matches []grepMatch) string {
	header := fmt.Sprintf("Found %d matches for %q under %s", len(matches), pattern, relPath)
	if len(matches) == 0 {
		return header + "\n\n(no matches found)\n"
	}
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n\n")
	currentFile := ""
	for _, match := range matches {
		if match.RelPath != currentFile {
			if currentFile != "" {
				b.WriteString("\n")
			}
			currentFile = match.RelPath
			b.WriteString(match.RelPath)
			b.WriteString(":\n")
		}
		b.WriteString(fmt.Sprintf(" Line %d: %s\n", match.Line, match.Text))
	}
	return b.String()
}

func globToRegex(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if ch == '*' {
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString(`[^/]*`)
			}
			continue
		}
		if ch == '?' {
			b.WriteString(".")
			continue
		}
		if strings.ContainsRune(`.+()|[]{}^$\\`, rune(ch)) {
			b.WriteByte('\\')
		}
		b.WriteByte(ch)
	}
	b.WriteString("$")
	return b.String()
}
