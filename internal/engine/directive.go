package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DirectiveDocument struct {
	SessionID       string
	CurrentCMOwner  string
	PreviousCMOwner string
	Version         int
	LastUpdated     time.Time
	Sections        map[string][]string
}

func (s *Service) UpdateDirective(workspacePath string, doc DirectiveDocument) error {
	if doc.Sections == nil {
		doc.Sections = map[string][]string{}
	}
	return writeText(filepath.Join(workspacePath, directiveFileName), renderDirectiveVersion(
		SessionFile{
			SessionID: doc.SessionID,
			UpdatedAt: doc.LastUpdated,
		},
		doc.CurrentCMOwner,
		doc.PreviousCMOwner,
		doc.Version,
		doc.Sections,
	))
}

func (s *Service) LoadDirective(workspacePath string) (DirectiveDocument, error) {
	path := filepath.Join(workspacePath, directiveFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return DirectiveDocument{}, fmt.Errorf("read directive %q: %w", path, err)
	}
	return parseDirective(string(data)), nil
}

func parseDirective(content string) DirectiveDocument {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	doc := DirectiveDocument{
		Sections: map[string][]string{},
	}

	var current string
	var subsection string
	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "- Session ID: "):
			if doc.SessionID == "" {
				doc.SessionID = strings.TrimPrefix(trimmed, "- Session ID: ")
			}
		case strings.HasPrefix(trimmed, "- Current CM Owner: "):
			if doc.CurrentCMOwner == "" {
				doc.CurrentCMOwner = strings.TrimPrefix(trimmed, "- Current CM Owner: ")
			}
		case strings.HasPrefix(trimmed, "- Previous CM Owner: "):
			if doc.PreviousCMOwner == "" {
				doc.PreviousCMOwner = strings.TrimPrefix(trimmed, "- Previous CM Owner: ")
			}
		case strings.HasPrefix(trimmed, "- Directive Version: "):
			fmt.Sscanf(strings.TrimPrefix(trimmed, "- Directive Version: "), "%d", &doc.Version)
		case strings.HasPrefix(trimmed, "## "):
			current = normalizeDirectiveHeading(trimmed)
			subsection = ""
			if _, ok := doc.Sections[current]; !ok {
				doc.Sections[current] = []string{}
			}
		case strings.HasPrefix(trimmed, "### "):
			subsection = normalizeDirectiveSubheading(trimmed)
			key := directiveCompositeKey(current, subsection)
			if _, ok := doc.Sections[key]; !ok {
				doc.Sections[key] = []string{}
			}
		case strings.HasPrefix(trimmed, "- "):
			key := current
			if subsection != "" {
				key = directiveCompositeKey(current, subsection)
			}
			doc.Sections[key] = append(doc.Sections[key], trimmed)
		}
	}

	return doc
}

func normalizeDirectiveHeading(heading string) string {
	switch heading {
	case "## Session Identity":
		return "session_identity"
	case "## User Request":
		return "user_request"
	case "## User Answers And Locked Decisions":
		return "locked_decisions"
	case "## Assumptions":
		return "assumptions"
	case "## Product Framing":
		return "product_framing"
	case "## Competitive / Reference Intelligence":
		return "competitive"
	case "## Scope Classification":
		return "scope"
	case "## Architecture Direction":
		return "architecture_direction"
	case "## Full Feature Inventory":
		return "full_feature_inventory"
	case "## Risks And Unknowns":
		return "risks_and_unknowns"
	case "## Backlog Strategy":
		return "backlog_strategy"
	case "## Dispatch Notes For Final CM":
		return "dispatch_notes_for_final_cm"
	default:
		return strings.TrimPrefix(heading, "## ")
	}
}

func normalizeDirectiveSubheading(heading string) string {
	switch heading {
	case "### Required For Category Credibility":
		return "required"
	case "### Best-In-Class Enhancements":
		return "best_in_class"
	case "### Stretch / Later":
		return "stretch"
	default:
		return strings.TrimPrefix(heading, "### ")
	}
}

func directiveCompositeKey(section, subsection string) string {
	return section + "." + subsection
}
