package guidance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"codedone/internal/engine"
)

const defaultDir = "guidance"

var validID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type Entry struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Tags    []string `json:"tags"`
	Summary string   `json:"summary"`
}

type ListArgs struct{}

type GetArgs struct {
	IDs []string `json:"ids"`
}

type Document struct {
	Entry   Entry  `json:"entry"`
	Content string `json:"content"`
}

func ExecuteList() ([]Entry, engine.ToolCall, error) {
	call := engine.ToolCall{Name: "guidance_list", Target: defaultDir}
	entries, err := LoadIndex()
	if err != nil {
		call.Status = "failed"
		return nil, call, err
	}
	call.Status = "done"
	return entries, call, nil
}

func ExecuteGet(args GetArgs) ([]Document, engine.ToolCall, error) {
	call := engine.ToolCall{Name: "guidance_get", Target: strings.Join(args.IDs, ",")}
	docs, err := LoadDocuments(args.IDs)
	if err != nil {
		call.Status = "failed"
		return nil, call, err
	}
	call.Status = "done"
	return docs, call, nil
}

func LoadIndex() ([]Entry, error) {
	data, err := os.ReadFile(filepath.Join(defaultDir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("read guidance index: %w", err)
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("decode guidance index: %w", err)
	}
	return entries, nil
}

func LoadDocuments(ids []string) ([]Document, error) {
	index, err := LoadIndex()
	if err != nil {
		return nil, err
	}
	entries := map[string]Entry{}
	for _, entry := range index {
		entries[entry.ID] = entry
	}
	docs := []Document{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !validID.MatchString(id) {
			return nil, fmt.Errorf("invalid guidance id %q", id)
		}
		entry, ok := entries[id]
		if !ok {
			return nil, fmt.Errorf("unknown guidance id %q", id)
		}
		data, err := os.ReadFile(filepath.Join(defaultDir, id+".md"))
		if err != nil {
			return nil, fmt.Errorf("read guidance %s: %w", id, err)
		}
		docs = append(docs, Document{
			Entry:   entry,
			Content: strings.TrimSpace(string(data)),
		})
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no guidance ids requested")
	}
	return docs, nil
}
