package signal

import (
	"testing"
	"time"
)

func TestSourceRegistryCoversAllConstants(t *testing.T) {
	// Every Source* constant must have a registry entry.
	constants := []string{
		SourceGitHub, SourceGitHubSignal, SourceHN, SourceReddit,
		SourceNPM, SourceCrates, SourceGmail, SourceCalendar,
		SourceRSS, SourceStackOverflow, SourceDevTo, SourceWebhook,
	}
	for _, name := range constants {
		if _, ok := sourceRegistry[name]; !ok {
			t.Errorf("source constant %q has no registry entry", name)
		}
	}
}

func TestGetSourceInfo(t *testing.T) {
	info, ok := GetSourceInfo(SourceGitHub)
	if !ok {
		t.Fatal("GetSourceInfo(github) returned false")
	}
	if info.Label != "GitHub" {
		t.Errorf("label = %q, want GitHub", info.Label)
	}
	if len(info.Kinds) == 0 {
		t.Error("github should have kinds")
	}

	_, ok = GetSourceInfo("nonexistent")
	if ok {
		t.Error("GetSourceInfo(nonexistent) should return false")
	}
}

func TestAllSourceInfo(t *testing.T) {
	all := AllSourceInfo()
	if len(all) != len(sourceRegistry) {
		t.Errorf("AllSourceInfo returned %d, want %d", len(all), len(sourceRegistry))
	}
	// Verify sorted.
	for i := 1; i < len(all); i++ {
		if all[i].Name < all[i-1].Name {
			t.Errorf("not sorted: %q before %q", all[i-1].Name, all[i].Name)
		}
	}
}

func TestRegisteredSourcesReturnsOnlyActive(t *testing.T) {
	scheduler := NewScheduler(nil, nil, nil, nil)
	scheduler.Register(&fakePoller{source: "github"}, 5*time.Minute)
	scheduler.Register(&fakePoller{source: "hn"}, 5*time.Minute)

	sources := scheduler.RegisteredSources()
	if len(sources) != 2 {
		t.Fatalf("got %d sources, want 2", len(sources))
	}

	names := make(map[string]bool)
	for _, s := range sources {
		names[s.Name] = true
	}
	if !names["github"] || !names["hn"] {
		t.Errorf("expected github and hn, got %v", names)
	}
}

func TestRegisteredSourcesEmpty(t *testing.T) {
	scheduler := NewScheduler(nil, nil, nil, nil)
	sources := scheduler.RegisteredSources()
	if len(sources) != 0 {
		t.Errorf("got %d sources for empty scheduler, want 0", len(sources))
	}
}

func TestRegisteredSourcesUnknownPoller(t *testing.T) {
	scheduler := NewScheduler(nil, nil, nil, nil)
	scheduler.Register(&fakePoller{source: "gitlab"}, 5*time.Minute)

	sources := scheduler.RegisteredSources()
	if len(sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(sources))
	}
	if sources[0].Name != "gitlab" {
		t.Errorf("name = %q, want gitlab", sources[0].Name)
	}
	if sources[0].Label != "gitlab" {
		t.Errorf("unknown source label = %q, want gitlab (fallback)", sources[0].Label)
	}
}

// Uses fakePoller defined in signal_test.go.
