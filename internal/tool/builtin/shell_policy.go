//go:build unix

package builtin

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

// DefaultMaxOutputBytes is the default cap for command output (100KB).
const DefaultMaxOutputBytes = 100 * 1024

// denyPattern matches dangerous commands that should never be executed.
type denyPattern struct {
	re     *regexp.Regexp
	reason string
}

// denyPatterns are always-rejected command patterns.
// Ported from Pub's shell-policy.ts + Plandex research.
var denyPatterns = []denyPattern{
	{regexp.MustCompile(`\brm\s+(-\w*r\w*\s+(-\w*f\w*\s+)?|(-\w*f\w*\s+)?-\w*r\w*\s+)/\s*$`), "rm -rf / is forbidden"},
	{regexp.MustCompile(`\brm\s+(-\w*r\w*\s+(-\w*f\w*\s+)?|(-\w*f\w*\s+)?-\w*r\w*\s+)/\s`), "rm -rf / is forbidden"},
	{regexp.MustCompile(`\b(shutdown|reboot|halt|poweroff|init\s+[06])\b`), "system power commands are forbidden"},
	{regexp.MustCompile(`\b(mkfs|fdisk|parted)\b`), "disk manipulation is forbidden"},
	{regexp.MustCompile(`\bdd\s+if=`), "disk manipulation is forbidden"},
	{regexp.MustCompile(`\bchmod\s+777\s+/`), "chmod 777 on system paths is forbidden"},
	{regexp.MustCompile(`\bchown\s+root\b`), "chown root is forbidden"},
	{regexp.MustCompile(`:\(\)\s*\{\s*:\|:&\s*\}\s*;`), "fork bomb detected"},
	{regexp.MustCompile(`\bgh\s+pr\s+merge\b`), "PR merge requires human approval"},
}

// checkDenyPatterns returns a non-empty reason if the command matches a deny pattern.
func checkDenyPatterns(command string) string {
	for _, dp := range denyPatterns {
		if dp.re.MatchString(command) {
			return dp.reason
		}
	}
	return ""
}

// readOnlyDenyPatterns block write/mutate operations for agents that deny file writes.
// These agents can still grep, find, cat, git log, gh pr view, go vet, etc.
var readOnlyDenyPatterns = []denyPattern{
	// Git mutations (but NOT git log, git diff, git status, git show, git blame, git worktree list).
	{regexp.MustCompile(`\bgit\s+checkout\s+-b\b`), "git branch creation denied (read-only agent)"},
	{regexp.MustCompile(`\bgit\s+commit\b`), "git commit denied (read-only agent)"},
	{regexp.MustCompile(`\bgit\s+push\b`), "git push denied (read-only agent)"},
	{regexp.MustCompile(`\bgit\s+merge\b`), "git merge denied (read-only agent)"},
	{regexp.MustCompile(`\bgit\s+rebase\b`), "git rebase denied (read-only agent)"},
	{regexp.MustCompile(`\bgit\s+reset\b`), "git reset denied (read-only agent)"},
	{regexp.MustCompile(`\bgit\s+cherry-pick\b`), "git cherry-pick denied (read-only agent)"},
	{regexp.MustCompile(`\bgit\s+stash\s+(pop|drop|clear)\b`), "git stash mutation denied (read-only agent)"},
	{regexp.MustCompile(`\bgit\s+tag\b`), "git tag denied (read-only agent)"},
	{regexp.MustCompile(`\bgit\s+branch\s+-[dDmM]\b`), "git branch delete/rename denied (read-only agent)"},
	// GitHub CLI mutations (but NOT gh pr view, gh pr list, gh pr diff, gh pr checks).
	{regexp.MustCompile(`\bgh\s+pr\s+create\b`), "PR creation denied (read-only agent)"},
	{regexp.MustCompile(`\bgh\s+pr\s+close\b`), "PR close denied (read-only agent)"},
	{regexp.MustCompile(`\bgh\s+pr\s+comment\b`), "PR comment denied (read-only agent)"},
	{regexp.MustCompile(`\bgh\s+pr\s+edit\b`), "PR edit denied (read-only agent)"},
	{regexp.MustCompile(`\bgh\s+pr\s+ready\b`), "PR ready denied (read-only agent)"},
	{regexp.MustCompile(`\bgh\s+issue\s+(close|create|edit|comment)\b`), "issue mutation denied (read-only agent)"},
	// File writes through shell (but NOT grep, cat, find, etc.).
	{regexp.MustCompile(`\bsed\s+-i\b`), "in-place file edit denied (read-only agent)"},
	{regexp.MustCompile(`\btee\s`), "file write via tee denied (read-only agent)"},
}

// checkReadOnlyDenyPatterns returns a reason if the command matches a read-only deny pattern.
func checkReadOnlyDenyPatterns(command string) string {
	for _, dp := range readOnlyDenyPatterns {
		if dp.re.MatchString(command) {
			return dp.reason
		}
	}
	return ""
}

// envAllowlist is the set of exact environment variable names safe for child processes.
var envAllowlist = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "SHELL": true,
	"TERM": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true,
	"GOPATH": true, "GOROOT": true, "GOBIN": true, "GOPROXY": true,
	"GOMODCACHE": true, "GOTOOLCHAIN": true,
	"EDITOR": true, "VISUAL": true,
	"TZ": true, "TMPDIR": true,
	"COLORTERM": true, "FORCE_COLOR": true,
	"SSH_AUTH_SOCK": true,
}

// envPrefixAllowlist allows any variable starting with these prefixes.
var envPrefixAllowlist = []string{
	"GIT_",
	"npm_",
	"NODE_",
	"CAIRN_",
	"CARGO_",
	"RUSTUP_",
	"PNPM_",
}

// ghEnvAllowlist allows specific GH_/GITHUB_ vars needed by tools.
// GH_TOKEN is a credential — intentionally passed so `gh` CLI works.
// Only vars listed here pass through; broad GITHUB_* prefix is NOT allowed.
var ghEnvAllowlist = map[string]bool{
	"GH_TOKEN":       true, // gh CLI authentication
	"GH_ORGS":        true, // org filter (non-secret)
	"GITHUB_ACTIONS": true, // CI detection flag
}

// filteredEnv returns os.Environ() filtered to safe variables only.
// Secrets are excluded except specific tokens needed by tools (e.g. GH_TOKEN for gh CLI).
func filteredEnv() []string {
	var result []string
	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if envAllowlist[key] || ghEnvAllowlist[key] {
			result = append(result, kv)
			continue
		}
		for _, prefix := range envPrefixAllowlist {
			if strings.HasPrefix(key, prefix) {
				result = append(result, kv)
				break
			}
		}
	}
	return result
}

// shellInfo holds the detected shell path and capabilities.
type shellInfo struct {
	path             string
	supportsPipefail bool
}

var (
	detectedShell   shellInfo
	detectShellOnce sync.Once
	shellBlacklist  = map[string]bool{"fish": true, "nu": true}
	pipefailShells  = map[string]bool{"bash": true, "zsh": true}
)

// detectShell finds the best available shell. Prefers $SHELL (if compatible),
// falls back to bash, then /bin/sh. Fish and nu are blacklisted due to
// incompatible syntax. Result is cached.
func detectShell() shellInfo {
	detectShellOnce.Do(func() {
		// Try $SHELL first.
		if userShell := os.Getenv("SHELL"); userShell != "" {
			base := shellBasename(userShell)
			if !shellBlacklist[base] {
				detectedShell = shellInfo{
					path:             userShell,
					supportsPipefail: pipefailShells[base],
				}
				return
			}
		}

		// Try bash.
		if bashPath, err := exec.LookPath("bash"); err == nil {
			detectedShell = shellInfo{path: bashPath, supportsPipefail: true}
			return
		}

		// Fallback to /bin/sh.
		detectedShell = shellInfo{path: "/bin/sh", supportsPipefail: false}
	})
	return detectedShell
}

// shellBasename returns the base name of a shell path without directory.
func shellBasename(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// truncateOutput caps output to maxBytes. If truncated, appends a notice
// with the number of bytes dropped. Returns the (possibly truncated) string
// and whether truncation occurred.
func truncateOutput(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, false
	}

	// Cut at nearest newline before limit to avoid splitting mid-line.
	cut := s[:maxBytes]
	if idx := strings.LastIndex(cut, "\n"); idx > maxBytes*3/4 {
		cut = cut[:idx]
	}

	dropped := len(s) - len(cut)
	return cut + "\n\n... [" + strings.TrimSpace(formatBytes(dropped)) + " truncated] ...\n", true
}

// formatBytes returns a human-readable byte count.
func formatBytes(b int) string {
	switch {
	case b >= 1024*1024:
		return strings.TrimRight(strings.TrimRight(
			formatFloat(float64(b)/1024/1024), "0"), ".") + " MB"
	case b >= 1024:
		return strings.TrimRight(strings.TrimRight(
			formatFloat(float64(b)/1024), "0"), ".") + " KB"
	default:
		return formatInt(b) + " bytes"
	}
}

func formatFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(
		func() string { s := make([]byte, 0, 8); return string(appendFloat(s, f)) }(), "0"), ".")
}

// Simpler approach — just use Sprintf-like formatting without importing fmt.
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func appendFloat(buf []byte, f float64) []byte {
	whole := int(f)
	frac := int((f - float64(whole)) * 10)
	buf = append(buf, []byte(formatInt(whole))...)
	buf = append(buf, '.')
	buf = append(buf, byte('0'+frac))
	return buf
}

// detectPipeOrRedirect returns the first pipe or redirect operator found
// in the command, or "" if none. Skips operators inside single/double quotes.
func detectPipeOrRedirect(command string) string {
	inSingle := false
	inDouble := false
	runes := []rune(command)

	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		// Track quote state.
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			continue
		}

		// Skip escaped characters.
		if ch == '\\' && i+1 < len(runes) {
			i++
			continue
		}

		// Detect >> (must check before >).
		if ch == '>' && i+1 < len(runes) && runes[i+1] == '>' {
			return ">>"
		}
		// Detect > (but not >>).
		if ch == '>' {
			return ">"
		}
		// Detect | (but not ||).
		if ch == '|' {
			if i+1 < len(runes) && runes[i+1] == '|' {
				i++ // skip ||
				continue
			}
			return "|"
		}
	}
	return ""
}
