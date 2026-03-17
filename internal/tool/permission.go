package tool

import (
	"path/filepath"
)

// PermissionAction determines what happens when a tool+path matches a rule.
type PermissionAction string

const (
	Allow PermissionAction = "allow"
	Ask   PermissionAction = "ask"
	Deny  PermissionAction = "deny"
)

// PermissionRule is a single rule in the permission set. Tool and Pattern
// support wildcards: "*" matches anything. Pattern uses filepath.Match glob syntax.
type PermissionRule struct {
	Tool    string           // tool name or "*"
	Pattern string           // file glob or "*"
	Action  PermissionAction // allow, ask, deny
}

// PermissionSet is an ordered list of permission rules. First match wins.
type PermissionSet struct {
	Rules []PermissionRule
}

// Evaluate checks the rules in order for the given tool name and file path.
// Returns the action of the first matching rule. If no rule matches, returns Ask
// as the safe default.
func (ps *PermissionSet) Evaluate(toolName, filePath string) PermissionAction {
	if ps == nil || len(ps.Rules) == 0 {
		return Allow // no rules configured = allow all (explicit rules required to restrict)
	}
	for _, rule := range ps.Rules {
		if matchTool(rule.Tool, toolName) && matchPattern(rule.Pattern, filePath) {
			return rule.Action
		}
	}
	return Ask
}

// matchTool checks if the rule's tool field matches the given tool name.
func matchTool(ruleTool, toolName string) bool {
	if ruleTool == "*" {
		return true
	}
	return ruleTool == toolName
}

// matchPattern checks if the rule's pattern matches the given file path.
// Uses filepath.Match for glob matching.
func matchPattern(rulePattern, filePath string) bool {
	if rulePattern == "*" {
		return true
	}
	if filePath == "" {
		return rulePattern == ""
	}
	// Try matching against the full path.
	if matched, _ := filepath.Match(rulePattern, filePath); matched {
		return true
	}
	// Also try matching against just the base name (e.g., "*.env" should match "/foo/.env").
	base := filepath.Base(filePath)
	matched, _ := filepath.Match(rulePattern, base)
	return matched
}
