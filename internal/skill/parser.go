package skill

import (
	"fmt"
	"os"
	"strings"
)

// Parse reads a SKILL.md file at the given path and returns a parsed Skill.
func Parse(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("skill: read %q: %w", path, err)
	}
	return ParseContent(data, path)
}

// ParseContent parses a SKILL.md from raw bytes. The location is recorded
// on the returned Skill for diagnostics.
func ParseContent(content []byte, location string) (*Skill, error) {
	text := string(content)

	// Split frontmatter from body. Frontmatter is delimited by "---" lines.
	fm, body, err := splitFrontmatter(text)
	if err != nil {
		return nil, fmt.Errorf("skill: parse %q: %w", location, err)
	}

	fields, err := parseFrontmatterFields(fm)
	if err != nil {
		return nil, fmt.Errorf("skill: parse frontmatter %q: %w", location, err)
	}

	sk := &Skill{
		Location:  location,
		Inclusion: OnDemand, // default
		Metadata:  make(map[string]any),
		Content:   body,
	}

	// Known fields.
	for key, val := range fields {
		switch key {
		case "name":
			sk.Name = val
		case "description":
			sk.Description = val
		case "inclusion":
			switch strings.TrimSpace(val) {
			case "always":
				sk.Inclusion = Always
			case "on-demand":
				sk.Inclusion = OnDemand
			default:
				sk.Inclusion = OnDemand
			}
		case "allowed-tools":
			sk.AllowedTools = parseCSV(val)
		case "disable-model-invocation":
			sk.DisableModel = parseBool(val)
		default:
			// Everything else goes into Metadata.
			sk.Metadata[key] = autoType(val)
		}
	}

	if sk.Name == "" {
		return nil, fmt.Errorf("skill: %q: missing required field 'name'", location)
	}
	if sk.Description == "" {
		return nil, fmt.Errorf("skill: %q: missing required field 'description'", location)
	}

	return sk, nil
}

// splitFrontmatter splits a document into frontmatter (between first pair of
// "---" delimiters) and body (everything after the closing "---").
func splitFrontmatter(text string) (frontmatter, body string, err error) {
	// The document must start with "---".
	trimmed := strings.TrimLeft(text, " \t")
	if !strings.HasPrefix(trimmed, "---") {
		return "", "", fmt.Errorf("no frontmatter delimiter found")
	}

	// Find the opening delimiter.
	start := strings.Index(text, "---")
	rest := text[start+3:]

	// Find the closing delimiter.
	end := strings.Index(rest, "\n---")
	if end == -1 {
		return "", "", fmt.Errorf("no closing frontmatter delimiter found")
	}

	frontmatter = rest[:end]

	// Body starts after the closing "---" and its trailing newline.
	afterClose := rest[end+4:] // len("\n---") == 4
	// Skip one optional newline after the closing delimiter line.
	if len(afterClose) > 0 && afterClose[0] == '\n' {
		afterClose = afterClose[1:]
	} else if strings.HasPrefix(afterClose, "\r\n") {
		afterClose = afterClose[2:]
	}
	body = afterClose

	return frontmatter, body, nil
}

// parseFrontmatterFields does simple key: value parsing per line.
// Handles quoted strings (single or double), booleans, etc.
func parseFrontmatterFields(fm string) (map[string]string, error) {
	fields := make(map[string]string)

	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first colon.
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue // skip lines without a colon
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		// Strip surrounding quotes.
		val = unquote(val)

		fields[key] = val
	}

	return fields, nil
}

// unquote removes matching surrounding quotes (single or double).
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseCSV splits a comma-separated string into trimmed, non-empty parts.
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

// parseBool interprets common boolean-like strings.
func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "1":
		return true
	default:
		return false
	}
}

// autoType attempts to infer a typed value from a string for Metadata.
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
