package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/avifenesh/cairn/internal/skill"
	"github.com/avifenesh/cairn/internal/tool"
)

// SkillSuggestion represents a recommended skill from the marketplace.
type SkillSuggestion struct {
	Slug        string    `json:"slug"`
	DisplayName string    `json:"displayName"`
	Summary     string    `json:"summary"`
	Reason      string    `json:"reason"` // WHY suggested
	Signal      string    `json:"signal"` // signal type that triggered
	Score       float64   `json:"score"`  // relevance score
	CreatedAt   time.Time `json:"createdAt"`
}

// SkillGapSignal represents a detected capability gap.
type SkillGapSignal struct {
	Type     string   // "error", "topic", "pattern"
	Keywords []string // search terms for marketplace
	Context  string   // human-readable reason
	Weight   float64  // importance multiplier
}

// SkillSuggestor collects signals and generates skill suggestions.
type SkillSuggestor struct {
	mu          sync.RWMutex
	suggestions []SkillSuggestion
	updatedAt   time.Time
	dismissed   map[string]bool // slugs dismissed by user
	logger      *slog.Logger
}

// NewSkillSuggestor creates a new suggestor.
func NewSkillSuggestor(logger *slog.Logger) *SkillSuggestor {
	if logger == nil {
		logger = slog.Default()
	}
	return &SkillSuggestor{
		dismissed: make(map[string]bool),
		logger:    logger,
	}
}

// Suggestions returns a copy of the current suggestion list.
func (s *SkillSuggestor) Suggestions() ([]SkillSuggestion, time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SkillSuggestion, len(s.suggestions))
	copy(out, s.suggestions)
	return out, s.updatedAt
}

// ClearStale clears suggestions when no gaps are detected.
func (s *SkillSuggestor) ClearStale() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.suggestions) > 0 {
		s.suggestions = nil
		s.updatedAt = time.Now()
	}
}

// Dismiss marks a slug as dismissed (won't be suggested again until cleared).
func (s *SkillSuggestor) Dismiss(slug string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dismissed[slug] = true
	// Remove from current suggestions.
	filtered := make([]SkillSuggestion, 0, len(s.suggestions))
	for _, sg := range s.suggestions {
		if sg.Slug != slug {
			filtered = append(filtered, sg)
		}
	}
	s.suggestions = filtered
	s.updatedAt = time.Now()
}

// CollectSignals analyzes recent agent activity to detect skill gaps.
func CollectSignals(ctx context.Context, journal *JournalStore, activity *ActivityStore, skills tool.SkillService) []SkillGapSignal {
	var signals []SkillGapSignal

	// 1. Check recent journal errors for capability-gap keywords.
	if journal != nil {
		entries, err := journal.Recent(ctx, 24*time.Hour)
		if err == nil {
			errorKeywords := extractErrorKeywords(entries)
			for _, kw := range errorKeywords {
				signals = append(signals, SkillGapSignal{
					Type:     "error",
					Keywords: []string{kw},
					Context:  "Errors in recent sessions mention: " + kw,
					Weight:   1.5,
				})
			}
		}
	}

	// 2. Check recent activity for topic patterns.
	if activity != nil {
		entries, err := activity.List(ctx, 20, 0, "")
		if err != nil {
			slog.Debug("skill suggest: failed to list activity", "error", err)
		}
		topicSignals := extractTopicSignals(entries)
		signals = append(signals, topicSignals...)
	}

	// 3. Filter out signals already covered by installed skills.
	if skills != nil {
		installed := skills.List()
		installedDescs := make([]string, 0, len(installed))
		for _, sk := range installed {
			installedDescs = append(installedDescs, strings.ToLower(sk.Description))
		}
		signals = filterCoveredSignals(signals, installedDescs)
	}

	return signals
}

// GenerateSuggestions queries the marketplace for skills matching detected gaps.
func (s *SkillSuggestor) GenerateSuggestions(ctx context.Context, signals []SkillGapSignal, marketplace *skill.MarketplaceClient, installed tool.SkillService) {
	if len(signals) == 0 || marketplace == nil {
		return
	}

	var suggestions []SkillSuggestion
	seen := make(map[string]bool)

	// Get installed skill names for filtering.
	var installedNames []string
	if installed != nil {
		for _, sk := range installed.List() {
			installedNames = append(installedNames, sk.Name)
		}
	}

	for _, sig := range signals {
		query := strings.Join(sig.Keywords, " ")
		if query == "" {
			continue
		}

		results, err := marketplace.Search(ctx, query, 5)
		if err != nil {
			s.logger.Debug("skill suggest: search failed", "query", query, "error", err)
			continue
		}

		for _, r := range results {
			if seen[r.Slug] {
				continue
			}
			// Skip if already installed.
			isInstalled := false
			for _, name := range installedNames {
				if name == r.Slug {
					isInstalled = true
					break
				}
			}
			if isInstalled {
				continue
			}

			s.mu.RLock()
			isDismissed := s.dismissed[r.Slug]
			s.mu.RUnlock()
			if isDismissed {
				continue
			}

			seen[r.Slug] = true
			suggestions = append(suggestions, SkillSuggestion{
				Slug:        r.Slug,
				DisplayName: r.DisplayName,
				Summary:     r.Summary,
				Reason:      sig.Context,
				Signal:      sig.Type,
				Score:       r.Score * sig.Weight,
				CreatedAt:   time.Now(),
			})
		}
	}

	// Sort by score, keep top 3.
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Score > suggestions[j].Score
	})
	if len(suggestions) > 3 {
		suggestions = suggestions[:3]
	}

	s.mu.Lock()
	s.suggestions = suggestions
	s.updatedAt = time.Now()
	s.mu.Unlock()

	if len(suggestions) > 0 {
		s.logger.Info("skill suggest: generated suggestions",
			"count", len(suggestions),
			"top", suggestions[0].Slug)
	}
}

// extractErrorKeywords pulls actionable keywords from journal errors.
func extractErrorKeywords(entries []*JournalEntry) []string {
	seen := make(map[string]bool)
	var keywords []string

	for _, e := range entries {
		for _, errStr := range e.Errors {
			lower := strings.ToLower(errStr)
			// Look for technology/tool mentions in errors.
			for _, kw := range knownTechKeywords {
				if strings.Contains(lower, kw) && !seen[kw] {
					seen[kw] = true
					keywords = append(keywords, kw)
				}
			}
		}
	}
	return keywords
}

// extractTopicSignals finds recurring topics in activity summaries.
func extractTopicSignals(entries []ActivityEntry) []SkillGapSignal {
	topicCounts := make(map[string]int)

	for _, e := range entries {
		lower := strings.ToLower(e.Summary + " " + e.Details)
		for _, kw := range knownTechKeywords {
			if strings.Contains(lower, kw) {
				topicCounts[kw]++
			}
		}
	}

	var signals []SkillGapSignal
	for topic, count := range topicCounts {
		if count >= 2 { // mentioned 2+ times in recent activity
			signals = append(signals, SkillGapSignal{
				Type:     "topic",
				Keywords: []string{topic},
				Context:  fmt.Sprintf("Mentioned %d times in recent activity: %s", count, topic),
				Weight:   1.0 + float64(count)*0.1,
			})
		}
	}
	return signals
}

// filterCoveredSignals removes signals that are already addressed by installed skill descriptions.
func filterCoveredSignals(signals []SkillGapSignal, installedDescs []string) []SkillGapSignal {
	var filtered []SkillGapSignal
	for _, sig := range signals {
		covered := false
		for _, desc := range installedDescs {
			for _, kw := range sig.Keywords {
				if strings.Contains(desc, strings.ToLower(kw)) {
					covered = true
					break
				}
			}
			if covered {
				break
			}
		}
		if !covered {
			filtered = append(filtered, sig)
		}
	}
	return filtered
}

// knownTechKeywords are technology names that indicate a potential skill gap.
var knownTechKeywords = []string{
	"docker", "kubernetes", "terraform", "ansible", "nginx",
	"react", "vue", "angular", "nextjs", "nuxt",
	"python", "java", "ruby", "php", "csharp",
	"postgres", "mysql", "mongodb", "redis", "elasticsearch",
	"aws", "azure", "gcloud", "cloudflare",
	"graphql", "grpc", "rest api",
	"ci/cd", "github actions", "jenkins",
	"testing", "playwright", "cypress", "jest",
	"security", "authentication", "oauth",
	"machine learning", "pytorch", "tensorflow",
	"pdf", "excel", "csv",
}
