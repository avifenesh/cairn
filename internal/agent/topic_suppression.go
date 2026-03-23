package agent

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// topicSuppressionThreshold is the number of recent actions mentioning the same
// topic before it gets suppressed. Prevents the orchestrator from obsessing over
// a single entity (PR, branch, task) across multiple ticks.
const topicSuppressionThreshold = 3

// topicRule pairs a compiled regex with its topic prefix for type-safe dispatch.
type topicRule struct {
	re     *regexp.Regexp
	prefix string // "pr", "issue", "branch", "task", "session", "cron"
}

// topicRules extracts entity references from action summaries/reasons.
// The prefix field determines the topic label — no inspection of regex source needed.
var topicRules = []topicRule{
	{regexp.MustCompile(`(?i)PR\s*#(\d+)`), "pr"},
	{regexp.MustCompile(`(?i)pull\s*request\s*#?(\d+)`), "pr"},
	{regexp.MustCompile(`(?i)issue\s*#(\d+)`), "issue"},
	{regexp.MustCompile(`(?i)(?:branch|worktree)\s+(\S+/\S+)`), "branch"},
	{regexp.MustCompile(`(?i)task\s+([a-f0-9]{8,})`), "task"},
	{regexp.MustCompile(`(?i)session\s+([a-z]+-[a-f0-9]{8,})`), "session"},
	{regexp.MustCompile(`(?i)cron[- ]job\s+["']?([^"'\s,]+)["']?`), "cron"},
}

// extractTopics finds entity references in a text string.
// Returns deduplicated, normalized topic strings like "pr:219", "branch:fix/reply".
func extractTopics(text string) []string {
	seen := make(map[string]bool)
	var topics []string

	for _, rule := range topicRules {
		for _, match := range rule.re.FindAllStringSubmatch(text, -1) {
			if len(match) < 2 || match[1] == "" {
				continue
			}
			// Normalize: lowercase entity to ensure consistent matching.
			entity := strings.ToLower(match[1])
			topic := rule.prefix + ":" + entity
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
		// Extract from summary and details independently to avoid cross-field stitching.
		seen := make(map[string]bool)
		for _, text := range []string{a.Summary, a.Details} {
			for _, t := range extractTopics(text) {
				if !seen[t] {
					seen[t] = true
					topicCounts[t]++
				}
			}
		}
	}

	var suppressed []string
	for topic, count := range topicCounts {
		if count >= topicSuppressionThreshold {
			suppressed = append(suppressed, topic)
		}
	}
	if len(suppressed) > 1 {
		sort.Strings(suppressed)
	}
	return suppressed
}

// instructionMentionsTopic checks whether a spawn instruction references any
// of the suppressed topics. Uses word-boundary-aware matching to prevent
// false positives (e.g., "pr:219" should not match "#2190").
func instructionMentionsTopic(instruction string, suppressed []string) string {
	lower := strings.ToLower(instruction)
	for _, topic := range suppressed {
		parts := strings.SplitN(topic, ":", 2)
		if len(parts) != 2 {
			continue
		}
		entity := parts[1] // already lowercased from extractTopics

		switch parts[0] {
		case "pr", "issue":
			// For numeric IDs, require a non-digit boundary after the number.
			if containsWithBoundary(lower, "#"+entity) ||
				containsWithBoundary(lower, "pr "+entity) ||
				containsWithBoundary(lower, "pr #"+entity) ||
				containsWithBoundary(lower, "pull request "+entity) ||
				containsWithBoundary(lower, "issue #"+entity) ||
				containsWithBoundary(lower, "issue "+entity) {
				return topic
			}
		case "branch":
			// Branch names are path-like; require word boundary (space/start/end).
			if containsWithBoundary(lower, entity) {
				return topic
			}
		default:
			// task IDs, session IDs, cron names — require boundary.
			if containsWithBoundary(lower, entity) {
				return topic
			}
		}
	}
	return ""
}

// containsWithBoundary checks if needle exists in haystack with non-alphanumeric
// boundaries (or string start/end) on both sides. Prevents "219" from matching "2190".
func containsWithBoundary(haystack, needle string) bool {
	idx := 0
	for {
		pos := strings.Index(haystack[idx:], needle)
		if pos < 0 {
			return false
		}
		absPos := idx + pos
		endPos := absPos + len(needle)

		// Check left boundary: start of string or non-alnum character.
		leftOK := absPos == 0 || !isAlnum(rune(haystack[absPos-1]))
		// Check right boundary: end of string or non-alnum character.
		rightOK := endPos >= len(haystack) || !isAlnum(rune(haystack[endPos]))

		if leftOK && rightOK {
			return true
		}
		// Advance past this match to find the next occurrence.
		idx = absPos + 1
		if idx >= len(haystack) {
			return false
		}
	}
}

func isAlnum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
