package skill

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Severity classifies the importance of a validation issue.
type Severity string

const (
	// SeverityError indicates a critical issue that prevents proper functioning.
	SeverityError Severity = "error"
	// SeverityWarning indicates a non-critical issue worth attention.
	SeverityWarning Severity = "warning"
)

// ValidationIssue describes a single problem found during skill validation.
type ValidationIssue struct {
	Severity Severity
	Message  string
}

// minDescriptionLength is the minimum recommended character count for a skill description.
const minDescriptionLength = 10

// Validate checks a parsed skill for common issues and returns any problems found.
// knownTools is the list of registered tool names used to verify allowed-tools references.
func Validate(sk *Skill, knownTools []string) []ValidationIssue {
	var issues []ValidationIssue

	// Build lookup set for known tools.
	known := make(map[string]bool, len(knownTools))
	for _, t := range knownTools {
		known[t] = true
	}

	// Check allowed-tools entries exist in knownTools.
	for _, t := range sk.AllowedTools {
		if !known[t] {
			issues = append(issues, ValidationIssue{
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("allowed-tools references unknown tool %q", t),
			})
		}
	}

	// Check pub.shell without disable-model-invocation.
	if !sk.DisableModel {
		for _, t := range sk.AllowedTools {
			if t == "pub.shell" {
				issues = append(issues, ValidationIssue{
					Severity: SeverityWarning,
					Message:  "skill allows 'pub.shell' without 'disable-model-invocation' being set to 'true', which is a security risk",
				})
				break
			}
		}
	}

	// Check empty or short description.
	if len(strings.TrimSpace(sk.Description)) < minDescriptionLength {
		issues = append(issues, ValidationIssue{
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("description is too short (%d chars, minimum recommended: %d)", len(strings.TrimSpace(sk.Description)), minDescriptionLength),
		})
	}

	// Check name matches directory basename.
	if sk.Location != "" {
		dir := filepath.Dir(sk.Location) // e.g., /path/to/skills/my-skill/SKILL.md -> /path/to/skills/my-skill
		base := filepath.Base(dir)
		if base != sk.Name {
			issues = append(issues, ValidationIssue{
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("name %q does not match directory basename %q", sk.Name, base),
			})
		}
	}

	return issues
}
