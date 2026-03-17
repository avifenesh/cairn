package skill

import (
	"fmt"
	"sort"
	"strings"
)

// InjectSkills builds a system prompt section from active skills.
//
// Always-inclusion skills are rendered with their full body.
// OnDemand skills are listed by description only, so the LLM can
// decide whether to request activation.
//
// The mode parameter is reserved for future per-mode filtering.
// tokenBudget limits total output size (rough: 1 token ~ 4 chars).
// A budget of 0 or negative means unlimited.
func InjectSkills(skills []*Skill, mode string, tokenBudget int) string {
	if len(skills) == 0 {
		return ""
	}

	charBudget := 0
	if tokenBudget > 0 {
		charBudget = tokenBudget * 4
	}

	// Separate by inclusion.
	var always, onDemand []*Skill
	for _, sk := range skills {
		switch sk.Inclusion {
		case Always:
			always = append(always, sk)
		default:
			onDemand = append(onDemand, sk)
		}
	}

	// Sort each group by name for deterministic output.
	sort.Slice(always, func(i, j int) bool { return always[i].Name < always[j].Name })
	sort.Slice(onDemand, func(i, j int) bool { return onDemand[i].Name < onDemand[j].Name })

	var b strings.Builder

	b.WriteString("## Active Skills\n\n")

	// Always-included skills: full body.
	for _, sk := range always {
		section := fmt.Sprintf("### %s\n%s\n\n", sk.Name, sk.Content)

		if charBudget > 0 && b.Len()+len(section) > charBudget {
			// Truncate this skill's content to fit within budget.
			remaining := charBudget - b.Len()
			if remaining > 0 {
				b.WriteString(section[:remaining])
			}
			return b.String()
		}

		b.WriteString(section)
	}

	// On-demand skills: description only.
	if len(onDemand) > 0 {
		header := "### Available Skills (ask to activate)\n"
		if charBudget > 0 && b.Len()+len(header) > charBudget {
			return b.String()
		}
		b.WriteString(header)

		for _, sk := range onDemand {
			line := fmt.Sprintf("- %s: %s\n", sk.Name, sk.Description)

			if charBudget > 0 && b.Len()+len(line) > charBudget {
				return b.String()
			}
			b.WriteString(line)
		}
	}

	return b.String()
}
