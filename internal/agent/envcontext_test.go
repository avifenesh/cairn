package agent

import (
	"strings"
	"testing"
)

func TestEnvContext_Format(t *testing.T) {
	e := &EnvContext{
		OS:      "linux",
		Shell:   "/bin/bash",
		User:    "ubuntu",
		Home:    "/home/ubuntu",
		Go:      "go1.25",
		GitUser: "avifenesh",
	}
	got := e.Format()
	if !strings.Contains(got, "## Environment") {
		t.Error("missing header")
	}
	if !strings.Contains(got, "linux") {
		t.Error("missing OS")
	}
	if !strings.Contains(got, "avifenesh") {
		t.Error("missing git user")
	}
}

func TestEnvContext_FormatNil(t *testing.T) {
	var e *EnvContext
	if got := e.Format(); got != "" {
		t.Errorf("nil Format() = %q, want empty", got)
	}
}

func TestEnvContext_FormatPartial(t *testing.T) {
	e := &EnvContext{
		OS:   "darwin",
		Home: "/Users/test",
	}
	got := e.Format()
	if !strings.Contains(got, "darwin") {
		t.Error("missing OS")
	}
	if strings.Contains(got, "Shell") {
		t.Error("should not include empty Shell")
	}
}
