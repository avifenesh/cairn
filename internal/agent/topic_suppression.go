package agent

import (
	"regexp"
	"sort"
	"strings"
)

// topicSuppressionThreshold is the number of recent actions mentioning the same
// topic before it gets suppressed. Prevents the orchestrator from obsessing over
// a single entity (PR, branch, task) across multiple ticks.
const topicSuppressionThreshold = 3

// topicPatterns extracts entity references from action summaries/reasons.
var topicPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)PR\s*#(\d+)`),                         // PR #219
	regexp.MustCompile(`(?i)pull\s*request\s*#?(\d+)`),            // pull request 219
	regexp.MustCompile(`(?i)issue\s*#(\d+)`),                      // issue #42
	regexp.MustCompile(`(?:branch|worktree)\s+(\S+/\S+)`),         // branch fix/reply-context
	regexp.MustCompile(`(?i)task\s+([a-f0-9]{8,})`),               // task 3cf8a86c...
	regexp.MustCompile(`(?i)session\s+([a-z]+-[a-f0-9]{8,})`),     // session loop-abc123
	regexp.MustCompile(`(?i)cron[- ]job\s+["']?([^"'\s,]+)["']?`), // cron job "name"
}

// extractTopics finds entity references in a text string.
// Returns deduplicated, normalized topic strings like "pr:219", "branch:fix/reply".
func extractTopics(text string) []string {
	seen := make(map[string]bool)
	var topics []string

	for _, pat := range topicPatterns {
		for _, match := range pat.FindAllStringSubmatch(text, -1) {
			if len(match) < 2 || match[1] == "" {
				continue
			}
			// Normalize: lowercase prefix + captured group.
			var topic string
			prefix := strings.ToLower(pat.String())
			switch {
			case strings.Contains(prefix, "pr") || strings.Contains(prefix, "pull"):
				topic = "pr:" + match[1]
			case strings.Contains(prefix, "issue"):
				topic = "issue:" + match[1]
			case strings.Contains(prefix, "branch") || strings.Contains(prefix, "worktree"):
				topic = "branch:" + match[1]
			case strings.Contains(prefix, "task"):
				topic = "task:" + match[1]
			case strings.Contains(prefix, "session"):
				topic = "session:" + match[1]
			case strings.Contains(prefix, "cron"):
				topic = "cron:" + match[1]
			default:
				topic = "entity:" + match[1]
			}
			if !seen[topic] {
				seen[topic] = true
				topics = append(topics, topic)
			}
		}
	}
	return topics
}

// detectSuppressedTopics analyzes recent actions and returns topics that have
// appeared in topicSuppressionThreshold or more distinct actions. These topics
// should be blocked from further spawning.
func detectSuppressedTopics(actions []ActivityEntry) []string {
	topicCounts := make(map[string]int)

	for _, a := range actions {
		// Extract from both summary and details to catch all references.
		text := a.Summary + " " + a.Details
		actionTopics := extractTopics(text)
		// Count each topic once per action (not per mention within an action).
		for _, t := range actionTopics {
			topicCounts[t]++
		}
	}

	var suppressed []string
	for topic, count := range topicCounts {
		if count >= topicSuppressionThreshold {
			suppressed = append(suppressed, topic)
		}
	}
	sort.Strings(suppressed)
	return suppressed
}

// instructionMentionsTopic checks whether a spawn instruction references any
// of the suppressed topics. Used for code-level enforcement in execute().
func instructionMentionsTopic(instruction string, suppressed []string) string {
	lower := strings.ToLower(instruction)
	for _, topic := range suppressed {
		parts := strings.SplitN(topic, ":", 2)
		if len(parts) != 2 {
			continue
		}
		entity := parts[1]
		// Check for the entity value in the instruction.
		// For PRs: match "#219" or "PR 219" or "PR #219"
		// For branches: match the branch name directly
		switch parts[0] {
		case "pr":
			if strings.Contains(lower, "#"+entity) ||
				strings.Contains(lower, "pr "+entity) ||
				strings.Contains(lower, "pr #"+entity) ||
				strings.Contains(lower, "pull request "+entity) {
				return topic
			}
		case "branch":
			if strings.Contains(lower, entity) {
				return topic
			}
		default:
			if strings.Contains(lower, entity) {
				return topic
			}
		}
	}
	return ""
}
