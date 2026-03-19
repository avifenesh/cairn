package builtin

import (
	"testing"
)

func TestGWSEnabled(t *testing.T) {
	orig := gwsConfig
	defer func() { gwsConfig = orig }()

	SetGWSConfig("")
	if GWSEnabled() {
		t.Error("expected disabled when no path")
	}

	SetGWSConfig("/usr/local/bin/gws")
	if !GWSEnabled() {
		t.Error("expected enabled when path set")
	}
}

func TestGWSToolCount(t *testing.T) {
	origGWS := gwsConfig
	origZai := zaiConfig.APIKey
	origZaiEnabled := zaiConfig.enabled.Load()
	defer func() {
		gwsConfig = origGWS
		zaiConfig.APIKey = origZai
		zaiConfig.enabled.Store(origZaiEnabled)
	}()

	// Base: Z.ai disabled, GWS disabled = 24 tools.
	SetGWSConfig("")
	zaiConfig.APIKey = ""
	zaiConfig.enabled.Store(false)
	base := len(All())

	// GWS enabled = base + 2.
	SetGWSConfig("/usr/local/bin/gws")
	withGWS := len(All())
	if withGWS != base+2 {
		t.Errorf("expected %d tools with GWS, got %d", base+2, withGWS)
	}
}

func TestGWSToolNames(t *testing.T) {
	origGWS := gwsConfig
	defer func() { gwsConfig = origGWS }()

	SetGWSConfig("/usr/local/bin/gws")

	tools := All()
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Name()] = true
	}

	for _, expected := range []string{"cairn.gwsQuery", "cairn.gwsExecute"} {
		if !names[expected] {
			t.Errorf("expected tool %q not found", expected)
		}
	}
}
