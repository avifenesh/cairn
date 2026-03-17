package tool

import (
	"testing"
)

func TestPermission_ExactMatch(t *testing.T) {
	ps := &PermissionSet{
		Rules: []PermissionRule{
			{Tool: "pub.readFile", Pattern: "*", Action: Allow},
		},
	}

	action := ps.Evaluate("pub.readFile", "/some/file.txt")
	if action != Allow {
		t.Fatalf("expected Allow, got %s", action)
	}
}

func TestPermission_Wildcard(t *testing.T) {
	ps := &PermissionSet{
		Rules: []PermissionRule{
			{Tool: "*", Pattern: "*", Action: Allow},
		},
	}

	action := ps.Evaluate("pub.shell", "/any/path")
	if action != Allow {
		t.Fatalf("expected Allow for wildcard tool, got %s", action)
	}

	action = ps.Evaluate("pub.deleteFile", "")
	if action != Allow {
		t.Fatalf("expected Allow for wildcard tool with empty path, got %s", action)
	}
}

func TestPermission_FilePattern(t *testing.T) {
	ps := &PermissionSet{
		Rules: []PermissionRule{
			{Tool: "pub.writeFile", Pattern: "*.env", Action: Deny},
			{Tool: "pub.writeFile", Pattern: "*", Action: Allow},
		},
	}

	// .env file should be denied.
	action := ps.Evaluate("pub.writeFile", "/project/.env")
	if action != Deny {
		t.Fatalf("expected Deny for .env file, got %s", action)
	}

	// Regular file should be allowed.
	action = ps.Evaluate("pub.writeFile", "/project/main.go")
	if action != Allow {
		t.Fatalf("expected Allow for .go file, got %s", action)
	}
}

func TestPermission_FirstMatchWins(t *testing.T) {
	ps := &PermissionSet{
		Rules: []PermissionRule{
			{Tool: "pub.shell", Pattern: "*", Action: Deny},
			{Tool: "pub.shell", Pattern: "*", Action: Allow},
		},
	}

	// Deny comes first, so it should win.
	action := ps.Evaluate("pub.shell", "/any")
	if action != Deny {
		t.Fatalf("expected Deny (first match), got %s", action)
	}
}

func TestPermission_DefaultAllow(t *testing.T) {
	// Empty rule set = allow all (no restrictions configured).
	ps := &PermissionSet{Rules: []PermissionRule{}}
	action := ps.Evaluate("pub.anything", "/some/file")
	if action != Allow {
		t.Fatalf("expected Allow for empty rules, got %s", action)
	}

	// Nil permission set = allow all.
	var nilPS *PermissionSet
	action = nilPS.Evaluate("pub.anything", "/some/file")
	if action != Allow {
		t.Fatalf("expected Allow for nil PermissionSet, got %s", action)
	}
}

func TestPermission_AskWithRules(t *testing.T) {
	// When rules exist but none match, default is Ask (safe).
	ps := &PermissionSet{Rules: []PermissionRule{
		{Tool: "pub.specific", Pattern: "*", Action: Allow},
	}}
	action := ps.Evaluate("pub.other", "/some/file")
	if action != Ask {
		t.Fatalf("expected Ask when rules exist but none match, got %s", action)
	}
}
