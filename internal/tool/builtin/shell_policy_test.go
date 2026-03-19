//go:build unix

package builtin

import (
	"os"
	"testing"
)

func TestCheckDenyPatterns(t *testing.T) {
	tests := []struct {
		command string
		denied  bool
	}{
		// Should be denied.
		{"rm -rf / ", true},
		{"rm -rf /", true},
		{"rm -rf / --no-preserve-root", true},
		{"rm -fr / ", true},
		{"sudo shutdown -h now", true},
		{"reboot", true},
		{"halt", true},
		{"poweroff", true},
		{"mkfs.ext4 /dev/sda1", true},
		{"dd if=/dev/zero of=/dev/sda", true},
		{"chmod 777 /etc", true},
		{"chown root file.txt", true},
		{":() { :|:& }; :", true},

		// Should be allowed.
		{"ls -la", false},
		{"rm file.txt", false},
		{"rm -rf ./build", false},
		{"rm -rf /tmp/test ", false}, // specific path, not root
		{"npm test", false},
		{"go build ./...", false},
		{"git status", false},
		{"chmod 755 script.sh", false},
		{"chown ubuntu file.txt", false},
		{"echo hello", false},
		{"cat /etc/hosts", false},
	}

	for _, tt := range tests {
		reason := checkDenyPatterns(tt.command)
		if tt.denied && reason == "" {
			t.Errorf("expected command to be denied: %q", tt.command)
		}
		if !tt.denied && reason != "" {
			t.Errorf("expected command to be allowed: %q, got reason: %s", tt.command, reason)
		}
	}
}

func TestFilteredEnv(t *testing.T) {
	// Set some test env vars.
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", "/home/test")
	t.Setenv("GLM_API_KEY", "secret-key")
	t.Setenv("OPENAI_API_KEY", "sk-secret")
	t.Setenv("GIT_AUTHOR_NAME", "Test User")
	t.Setenv("npm_config_registry", "https://registry.npmjs.org")
	t.Setenv("CAIRN_DATA_DIR", "/data")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "supersecret")
	t.Setenv("DATABASE_PASSWORD", "dbpass")

	env := filteredEnv()
	envMap := make(map[string]string)
	for _, kv := range env {
		k, v, _ := splitEnv(kv)
		envMap[k] = v
	}

	// Should be present.
	for _, key := range []string{"PATH", "HOME", "GIT_AUTHOR_NAME", "npm_config_registry", "CAIRN_DATA_DIR"} {
		if _, ok := envMap[key]; !ok {
			t.Errorf("expected %s to be in filtered env", key)
		}
	}

	// Should be absent (secrets).
	for _, key := range []string{"GLM_API_KEY", "OPENAI_API_KEY", "AWS_SECRET_ACCESS_KEY", "DATABASE_PASSWORD"} {
		if _, ok := envMap[key]; ok {
			t.Errorf("expected %s to be filtered out", key)
		}
	}
}

func splitEnv(kv string) (string, string, bool) {
	for i, c := range kv {
		if c == '=' {
			return kv[:i], kv[i+1:], true
		}
	}
	return kv, "", false
}

func TestDetectShell(t *testing.T) {
	info := detectShell()
	if info.path == "" {
		t.Fatal("detectShell returned empty path")
	}
	// Verify the shell binary exists.
	if _, err := os.Stat(info.path); err != nil {
		t.Fatalf("detected shell %q does not exist: %v", info.path, err)
	}
}

func TestTruncateOutput(t *testing.T) {
	// Under limit — no truncation.
	short := "hello world"
	result, truncated := truncateOutput(short, 100)
	if truncated {
		t.Error("expected no truncation for short string")
	}
	if result != short {
		t.Errorf("expected %q, got %q", short, result)
	}

	// Over limit — truncated.
	long := make([]byte, 1024)
	for i := range long {
		if i%80 == 79 {
			long[i] = '\n'
		} else {
			long[i] = 'x'
		}
	}
	result, truncated = truncateOutput(string(long), 200)
	if !truncated {
		t.Error("expected truncation for long string")
	}
	if len(result) > 300 { // some slack for the notice
		t.Errorf("truncated result too long: %d bytes", len(result))
	}
	if !containsSubstring(result, "truncated") {
		t.Error("expected truncation notice in output")
	}

	// Zero limit — no truncation.
	result, truncated = truncateOutput(string(long), 0)
	if truncated {
		t.Error("expected no truncation with 0 limit")
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && searchSubstring(s, sub)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestDetectPipeOrRedirect(t *testing.T) {
	tests := []struct {
		command  string
		expected string
	}{
		{"ls -la", ""},
		{"echo hello", ""},
		{"ls | grep foo", "|"},
		{"echo hello > file.txt", ">"},
		{"echo hello >> file.txt", ">>"},
		// Quoted pipes should not trigger.
		{`echo "hello | world"`, ""},
		{`echo 'pipe | here'`, ""},
		// || is logical OR, not pipe.
		{"test -f file || echo missing", ""},
		// && is fine.
		{"make && make test", ""},
		// Escaped pipe.
		{`echo hello \| world`, ""},
	}

	for _, tt := range tests {
		got := detectPipeOrRedirect(tt.command)
		if got != tt.expected {
			t.Errorf("detectPipeOrRedirect(%q) = %q, want %q", tt.command, got, tt.expected)
		}
	}
}
