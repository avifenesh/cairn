package signal

import "strings"

// defaultBots is the hardcoded list of known bot accounts on GitHub.
var defaultBots = map[string]bool{
	"dependabot[bot]":              true,
	"dependabot":                   true,
	"github-actions[bot]":          true,
	"github-actions":               true,
	"copilot":                      true,
	"copilot[bot]":                 true,
	"gemini-code-assist[bot]":      true,
	"chatgpt-codex-connector[bot]": true,
	"claude-review[bot]":           true,
	"renovate[bot]":                true,
	"renovate":                     true,
	"snyk-bot":                     true,
	"snyk[bot]":                    true,
	"codecov[bot]":                 true,
	"stale[bot]":                   true,
	"allcontributors[bot]":         true,
	"gitguardian[bot]":             true,
	"sonarcloud[bot]":              true,
	"mergify[bot]":                 true,
	"netlify[bot]":                 true,
	"vercel[bot]":                  true,
	"greenkeeper[bot]":             true,
}

// IsBot returns true if the login is a known bot account.
// Checks the default list, extra custom logins, and common patterns.
func IsBot(login string, extra map[string]bool) bool {
	lower := strings.ToLower(login)
	if defaultBots[lower] {
		return true
	}
	if extra != nil && extra[lower] {
		return true
	}
	if strings.HasSuffix(lower, "[bot]") {
		return true
	}
	if strings.HasSuffix(lower, "-bot") {
		return true
	}
	return false
}
