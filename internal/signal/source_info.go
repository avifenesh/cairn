package signal

import "sort"

// SourceInfo describes a registered signal source and the event metadata it produces.
type SourceInfo struct {
	Name   string   `json:"name"`   // source identifier (e.g. "github")
	Label  string   `json:"label"`  // human-readable label (e.g. "GitHub")
	Kinds  []string `json:"kinds"`  // event kinds this source produces
	Fields []string `json:"fields"` // filterable fields in the EventIngested data map
}

// defaultFields returns the data map keys available on every EventIngested event.
// Returns a fresh copy each time to prevent callers from mutating shared state.
func defaultFields() []string {
	return []string{"sourceType", "kind", "title", "url", "actor", "repo"}
}

// sourceRegistry maps source name → metadata. Every Source* constant should have an entry.
var sourceRegistry = map[string]SourceInfo{
	SourceGitHub: {
		Name:   SourceGitHub,
		Label:  "GitHub",
		Kinds:  []string{KindPR, KindIssue, KindRelease, KindDiscussion, KindCommit, KindPush, KindBranch, KindFork, KindStar},
		Fields: defaultFields(),
	},
	SourceGitHubSignal: {
		Name:   SourceGitHubSignal,
		Label:  "GitHub Signal",
		Kinds:  []string{KindMetrics, KindFollow, KindStar, KindNewRepo},
		Fields: defaultFields(),
	},
	SourceHN: {
		Name:   SourceHN,
		Label:  "Hacker News",
		Kinds:  []string{KindStory},
		Fields: defaultFields(),
	},
	SourceReddit: {
		Name:   SourceReddit,
		Label:  "Reddit",
		Kinds:  []string{KindPost},
		Fields: defaultFields(),
	},
	SourceNPM: {
		Name:   SourceNPM,
		Label:  "npm",
		Kinds:  []string{KindMetrics, KindPackage},
		Fields: defaultFields(),
	},
	SourceCrates: {
		Name:   SourceCrates,
		Label:  "crates.io",
		Kinds:  []string{KindMetrics, KindPackage},
		Fields: defaultFields(),
	},
	SourceGmail: {
		Name:   SourceGmail,
		Label:  "Gmail",
		Kinds:  []string{KindEmail},
		Fields: defaultFields(),
	},
	SourceCalendar: {
		Name:   SourceCalendar,
		Label:  "Calendar",
		Kinds:  []string{KindEvent, KindInvitation},
		Fields: defaultFields(),
	},
	SourceRSS: {
		Name:   SourceRSS,
		Label:  "RSS",
		Kinds:  []string{KindStory, KindPost, KindRelease},
		Fields: defaultFields(),
	},
	SourceStackOverflow: {
		Name:   SourceStackOverflow,
		Label:  "Stack Overflow",
		Kinds:  []string{KindPost},
		Fields: defaultFields(),
	},
	SourceDevTo: {
		Name:   SourceDevTo,
		Label:  "Dev.to",
		Kinds:  []string{KindPost},
		Fields: defaultFields(),
	},
	SourceWebhook: {
		Name:   SourceWebhook,
		Label:  "Webhook",
		Kinds:  []string{KindWebhook},
		Fields: defaultFields(),
	},
}

// GetSourceInfo returns metadata for a source by name.
func GetSourceInfo(name string) (SourceInfo, bool) {
	info, ok := sourceRegistry[name]
	return info, ok
}

// AllSourceInfo returns metadata for all known sources, sorted by name.
func AllSourceInfo() []SourceInfo {
	result := make([]SourceInfo, 0, len(sourceRegistry))
	for _, info := range sourceRegistry {
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// RegisteredSources returns metadata for only the sources that have active pollers.
func (s *Scheduler) RegisteredSources() []SourceInfo {
	seen := make(map[string]bool, len(s.pollers))
	var result []SourceInfo
	for i := range s.pollers {
		name := s.pollers[i].poller.Source()
		if seen[name] {
			continue
		}
		seen[name] = true
		if info, ok := sourceRegistry[name]; ok {
			result = append(result, info)
		} else {
			// Unknown source — return minimal info so the API still lists it.
			result = append(result, SourceInfo{
				Name:   name,
				Label:  name,
				Fields: defaultFields(),
			})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
