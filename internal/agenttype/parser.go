package agenttype

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/avifenesh/cairn/internal/tool"
)

// Parse reads an AGENT.md file at the given path and returns a parsed AgentType.
func Parse(path string) (*AgentType, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("agenttype: read %q: %w", path, err)
	}
	return ParseContent(data, path)
}

// ParseContent parses an AGENT.md from raw bytes.
func ParseContent(content []byte, location string) (*AgentType, error) {
	text := string(content)

	fm, body, err := splitFrontmatter(text)
	if err != nil {
		return nil, fmt.Errorf("agenttype: parse %q: %w", location, err)
	}

	fields, err := parseFrontmatterFields(fm)
	if err != nil {
		return nil, fmt.Errorf("agenttype: parse frontmatter %q: %w", location, err)
	}

	at := &AgentType{
		Location: location,
		Mode:     tool.ModeWork, // default
		Metadata: make(map[string]any),
		Content:  body,
	}

	for key, val := range fields {
		switch key {
		case "name":
			at.Name = val
		case "description":
			at.Description = val
		case "mode":
			switch strings.TrimSpace(strings.ToLower(val)) {
			case "talk":
				at.Mode = tool.ModeTalk
			case "work":
				at.Mode = tool.ModeWork
			case "coding":
				at.Mode = tool.ModeCoding
			default:
				at.Mode = tool.ModeWork
			}
		case "allowed-tools":
			at.AllowedTools = parseCSV(val)
		case "denied-tools":
			at.DeniedTools = parseCSV(val)
		case "max-rounds":
			if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && n > 0 {
				at.MaxRounds = n
			}
		case "model":
			at.Model = val
		case "worktree":
			at.Worktree = parseBool(val)
		default:
			at.Metadata[key] = autoType(val)
		}
	}

	if at.Name == "" {
		return nil, fmt.Errorf("agenttype: %q: missing required field 'name'", location)
	}
	if at.MaxRounds <= 0 {
		at.MaxRounds = 20 // sensible default
	}

	return at, nil
}

// --- Frontmatter helpers (copied from skill/parser.go) ---

func splitFrontmatter(text string) (frontmatter, body string, err error) {
	trimmed := strings.TrimLeft(text, " \t")
	if !strings.HasPrefix(trimmed, "---") {
		return "", "", fmt.Errorf("no frontmatter delimiter found")
	}

	start := strings.Index(text, "---")
	rest := text[start+3:]

	end := strings.Index(rest, "\n---")
	if end == -1 {
		return "", "", fmt.Errorf("no closing frontmatter delimiter found")
	}

	frontmatter = rest[:end]

	afterClose := rest[end+4:]
	if len(afterClose) > 0 && afterClose[0] == '\n' {
		afterClose = afterClose[1:]
	} else if strings.HasPrefix(afterClose, "\r\n") {
		afterClose = afterClose[2:]
	}
	body = afterClose

	return frontmatter, body, nil
}

func parseFrontmatterFields(fm string) (map[string]string, error) {
	fields := make(map[string]string)

	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = unquote(val)

		fields[key] = val
	}

	return fields, nil
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func parseCSV(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "1":
		return true
	default:
		return false
	}
}

func autoType(s string) any {
	lower := strings.ToLower(strings.TrimSpace(s))
	switch lower {
	case "true", "yes":
		return true
	case "false", "no":
		return false
	}
	return s
}
