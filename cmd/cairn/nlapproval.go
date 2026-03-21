package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	cairnchannel "github.com/avifenesh/cairn/internal/channel"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/task"
)

// ApprovalAction represents the user's intended action.
type ApprovalAction int

const (
	ActionApprove ApprovalAction = iota
	ActionDeny
	ActionShow
)

// ApprovalTarget represents what the user wants to act on.
type ApprovalTarget int

const (
	TargetUnknown ApprovalTarget = iota
	TargetMemory
	TargetSoulPatch
	TargetApproval
)

// ApprovalIntent is the parsed result of natural language approval text.
type ApprovalIntent struct {
	Action   ApprovalAction
	Target   ApprovalTarget
	TargetID string // extracted ID or prefix from the text
	All      bool   // "approve all" pattern
}

// Keyword lists ordered by specificity (multi-word phrases first).
var (
	approvePhrasesMulti = []string{
		"looks good", "go ahead", "ship it", "apply it", "do it",
	}
	denyPhrasesMulti = []string{
		"drop it",
	}
	showPhrasesMulti = []string{
		"what's pending", "whats pending", "show pending",
		"list pending", "what do you have", "pending items",
	}

	targetMemoryPhrases    = []string{"memories", "memory", "mem_"}
	targetSoulPatchPhrases = []string{"soul patch", "soul", "patch"}
	targetApprovalPhrases  = []string{"approval", "apr_"}

	// Multi-word "all" phrases use substring matching; single-word "all"
	// uses whole-word matching to avoid false positives like "allocated".
	allPhrasesMulti  = []string{"all of them", "all proposed"}
	allPhrasesSingle = []string{"everything", "all"}

	// idPattern matches memory/soul/approval IDs or bare hex prefixes.
	idPattern = regexp.MustCompile(`\b(?:mem_|sp_|apr_)[a-f0-9]+\b|\b[a-f0-9]{8,24}\b`)
)

// parseApprovalIntent attempts to parse natural language text as an approval intent.
// Returns nil if the text is not an approval-related message.
func parseApprovalIntent(text string) *ApprovalIntent {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return nil
	}

	// Extract ID first — a bare ID reply (e.g. "a1b2c3d4") after a listing
	// is a valid approval intent even without an action keyword.
	var targetID string
	if match := idPattern.FindString(lower); match != "" {
		targetID = match
	}

	// Detect action.
	action, hasAction, ambiguous := detectAction(lower)

	// If no action keyword but a bare ID was provided, treat as approve
	// (the user is replying to a "which one?" listing).
	if !hasAction {
		if targetID != "" {
			action = ActionApprove
			hasAction = true
			ambiguous = false
		} else {
			return nil
		}
	}

	// Detect target.
	target := detectTarget(lower)
	hasAll := containsAny(lower, allPhrasesMulti) || containsAnyWord(lower, allPhrasesSingle)

	// Infer target from ID prefix.
	if targetID != "" && target == TargetUnknown {
		switch {
		case strings.HasPrefix(targetID, "mem_"):
			target = TargetMemory
		case strings.HasPrefix(targetID, "sp_"):
			target = TargetSoulPatch
		case strings.HasPrefix(targetID, "apr_"):
			target = TargetApproval
		}
	}

	// Ambiguous short words ("pass", "skip", "no", "yes", "ok", etc.) in longer
	// sentences are likely conversational, not approval intents. Only treat them
	// as intents if: the message is very short (<=2 words), OR a target/ID/all is present.
	if ambiguous && target == TargetUnknown && targetID == "" && !hasAll {
		wordCount := len(strings.Fields(lower))
		if wordCount > 2 {
			return nil
		}
	}

	return &ApprovalIntent{
		Action:   action,
		Target:   target,
		TargetID: targetID,
		All:      hasAll,
	}
}

// detectAction returns (action, found, ambiguous).
// ambiguous=true when the match is a short word that could be conversational.
func detectAction(lower string) (ApprovalAction, bool, bool) {
	// Check multi-word phrases first (more specific, never ambiguous).
	if containsAny(lower, showPhrasesMulti) {
		return ActionShow, true, false
	}
	if containsAny(lower, approvePhrasesMulti) {
		return ActionApprove, true, false
	}
	if containsAny(lower, denyPhrasesMulti) {
		return ActionDeny, true, false
	}
	// Strong single-word keywords (unambiguous).
	strongApprove := []string{"approve", "accept", "confirm", "confirmed", "lgtm"}
	strongDeny := []string{"deny", "reject", "decline", "discard", "cancel"}
	if containsAnyWord(lower, strongApprove) {
		return ActionApprove, true, false
	}
	if containsAnyWord(lower, strongDeny) {
		return ActionDeny, true, false
	}
	// Ambiguous single-word keywords (common in conversation).
	ambiguousApprove := []string{"yes", "yep", "yeah", "sure", "ok", "okay"}
	ambiguousDeny := []string{"nope", "nah", "pass", "skip", "no"}
	if containsAnyWord(lower, ambiguousApprove) {
		return ActionApprove, true, true
	}
	if containsAnyWord(lower, ambiguousDeny) {
		return ActionDeny, true, true
	}
	return 0, false, false
}

func detectTarget(lower string) ApprovalTarget {
	// Check multi-word target phrases first.
	if containsAny(lower, targetSoulPatchPhrases[:1]) { // "soul patch"
		return TargetSoulPatch
	}
	if containsAny(lower, targetMemoryPhrases) {
		return TargetMemory
	}
	// "soul" and "patch" individually — only if "soul patch" didn't match.
	if containsAny(lower, targetSoulPatchPhrases[1:]) {
		return TargetSoulPatch
	}
	if containsAny(lower, targetApprovalPhrases) {
		return TargetApproval
	}
	return TargetUnknown
}

// containsAny checks if text contains any of the phrases as substrings.
func containsAny(text string, phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}

// containsAnyWord checks if text contains any of the words as whole words.
// This prevents "notable" from matching "no", or "password" from matching "pass".
func containsAnyWord(text string, words []string) bool {
	fields := strings.Fields(text)
	for _, w := range words {
		for _, f := range fields {
			// Strip common trailing punctuation for matching.
			clean := strings.TrimRight(f, ".,!?;:")
			if clean == w {
				return true
			}
		}
	}
	return false
}

// pendingItem is a unified view of any pending approvable item.
type pendingItem struct {
	kind    string // "memory", "soul_patch", "approval"
	id      string
	display string // short summary for listing
}

// gatherPending collects all pending items across memories, soul, and approvals.
// Returns an error if any service call fails, to avoid acting on partial data.
func gatherPending(ctx context.Context, memSvc *memory.Service, soul *memory.Soul, approvals *task.ApprovalStore) ([]pendingItem, error) {
	var items []pendingItem

	if memSvc != nil {
		mems, err := memSvc.List(ctx, memory.ListOpts{Status: memory.StatusProposed, Limit: 50})
		if err != nil {
			return nil, fmt.Errorf("list proposed memories: %w", err)
		}
		for _, m := range mems {
			snippet := m.Content
			if len(snippet) > 60 {
				snippet = snippet[:57] + "..."
			}
			items = append(items, pendingItem{
				kind:    "memory",
				id:      m.ID,
				display: fmt.Sprintf("`%s` [%s] %s", shortID(m.ID), m.Category, snippet),
			})
		}
	}

	if soul != nil {
		if p := soul.PendingPatch(); p != nil {
			snippet := p.Content
			if len(snippet) > 60 {
				snippet = snippet[:57] + "..."
			}
			items = append(items, pendingItem{
				kind:    "soul_patch",
				id:      p.ID,
				display: fmt.Sprintf("`%s` [soul patch] %s", shortID(p.ID), snippet),
			})
		}
	}

	if approvals != nil {
		pending, err := approvals.ListPending(ctx)
		if err != nil {
			return nil, fmt.Errorf("list pending approvals: %w", err)
		}
		for _, a := range pending {
			snippet := a.Description
			if len(snippet) > 60 {
				snippet = snippet[:57] + "..."
			}
			items = append(items, pendingItem{
				kind:    "approval",
				id:      a.ID,
				display: fmt.Sprintf("`%s` [%s] %s", shortID(a.ID), a.Type, snippet),
			})
		}
	}

	return items, nil
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// handleApprovalIntent resolves and executes a parsed approval intent.
// channelName identifies where the decision came from (e.g. "telegram", "discord").
func handleApprovalIntent(
	ctx context.Context,
	intent *ApprovalIntent,
	memSvc *memory.Service,
	soul *memory.Soul,
	approvals *task.ApprovalStore,
	channelName string,
) (*cairnchannel.OutgoingMessage, error) {
	if intent.Action == ActionShow {
		return showPending(ctx, memSvc, soul, approvals)
	}

	// "approve all" / "reject all" with unknown target defaults to memories
	// (soul patches and approvals are higher-stakes, one-at-a-time).
	if intent.All && intent.Target == TargetUnknown {
		intent.Target = TargetMemory
	}

	switch intent.Target {
	case TargetMemory:
		return handleMemoryIntent(ctx, intent, memSvc)
	case TargetSoulPatch:
		return handleSoulPatchIntent(ctx, intent, soul)
	case TargetApproval:
		return handleGenericApprovalIntent(ctx, intent, approvals, channelName)
	default:
		return handleUnknownTarget(ctx, intent, memSvc, soul, approvals, channelName)
	}
}

func showPending(ctx context.Context, memSvc *memory.Service, soul *memory.Soul, approvals *task.ApprovalStore) (*cairnchannel.OutgoingMessage, error) {
	items, err := gatherPending(ctx, memSvc, soul, approvals)
	if err != nil {
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error gathering pending items: %s", err)}, nil
	}
	if len(items) == 0 {
		return &cairnchannel.OutgoingMessage{Text: "Nothing pending."}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**Pending items** (%d):\n\n", len(items))
	for i, it := range items {
		fmt.Fprintf(&b, "%d. %s\n", i+1, it.display)
	}
	b.WriteString("\nReply with the ID to approve/deny.")
	return &cairnchannel.OutgoingMessage{Text: b.String()}, nil
}

func handleMemoryIntent(ctx context.Context, intent *ApprovalIntent, memSvc *memory.Service) (*cairnchannel.OutgoingMessage, error) {
	if memSvc == nil {
		return &cairnchannel.OutgoingMessage{Text: "Memory service not available."}, nil
	}

	// Bulk approve or deny all proposed memories.
	if intent.All {
		mems, err := memSvc.List(ctx, memory.ListOpts{Status: memory.StatusProposed, Limit: 200})
		if err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error: %s", err)}, nil
		}
		if len(mems) == 0 {
			return &cairnchannel.OutgoingMessage{Text: "No proposed memories."}, nil
		}
		acted := 0
		verb := "Accepted"
		for _, m := range mems {
			var opErr error
			if intent.Action == ActionApprove {
				opErr = memSvc.Accept(ctx, m.ID)
			} else {
				opErr = memSvc.Reject(ctx, m.ID)
				verb = "Rejected"
			}
			if opErr == nil {
				acted++
			}
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("%s %d/%d proposed memories.", verb, acted, len(mems))}, nil
	}

	// Specific ID provided.
	if intent.TargetID != "" {
		id, err := resolveMemoryID(ctx, memSvc, intent.TargetID)
		if err != nil {
			return &cairnchannel.OutgoingMessage{Text: err.Error()}, nil
		}
		return applyMemoryAction(ctx, memSvc, id, intent.Action)
	}

	// No ID — check how many proposed memories exist.
	mems, err := memSvc.List(ctx, memory.ListOpts{Status: memory.StatusProposed, Limit: 50})
	if err != nil {
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error: %s", err)}, nil
	}
	if len(mems) == 0 {
		return &cairnchannel.OutgoingMessage{Text: "No proposed memories."}, nil
	}
	if len(mems) == 1 {
		return applyMemoryAction(ctx, memSvc, mems[0].ID, intent.Action)
	}
	// Multiple — list and ask.
	var b strings.Builder
	fmt.Fprintf(&b, "**%d proposed memories** — which one?\n\n", len(mems))
	for _, m := range mems {
		snippet := m.Content
		if len(snippet) > 60 {
			snippet = snippet[:57] + "..."
		}
		fmt.Fprintf(&b, "`%s` [%s] %.0f%% — %s\n", shortID(m.ID), m.Category, m.Confidence*100, snippet)
	}
	b.WriteString("\nReply with an ID, or say 'approve all' / 'reject all'.")
	return &cairnchannel.OutgoingMessage{Text: b.String()}, nil
}

func applyMemoryAction(ctx context.Context, memSvc *memory.Service, id string, action ApprovalAction) (*cairnchannel.OutgoingMessage, error) {
	sid := shortID(id)
	switch action {
	case ActionApprove:
		if err := memSvc.Accept(ctx, id); err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error accepting `%s`: %s", sid, err)}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Memory `%s` accepted.", sid)}, nil
	case ActionDeny:
		if err := memSvc.Reject(ctx, id); err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error rejecting `%s`: %s", sid, err)}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Memory `%s` rejected.", sid)}, nil
	default:
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Memory `%s` is pending.", sid)}, nil
	}
}

func handleSoulPatchIntent(ctx context.Context, intent *ApprovalIntent, soul *memory.Soul) (*cairnchannel.OutgoingMessage, error) {
	if soul == nil {
		return &cairnchannel.OutgoingMessage{Text: "Soul not configured."}, nil
	}
	p := soul.PendingPatch()
	if p == nil {
		return &cairnchannel.OutgoingMessage{Text: "No pending soul patch."}, nil
	}
	sid := shortID(p.ID)
	switch intent.Action {
	case ActionApprove:
		if err := soul.ApprovePatch(p.ID); err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error approving soul patch: %s", err)}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Soul patch `%s` approved and applied.", sid)}, nil
	case ActionDeny:
		if err := soul.DenyPatch(p.ID); err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error denying soul patch: %s", err)}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Soul patch `%s` denied.", sid)}, nil
	default:
		snippet := p.Content
		if len(snippet) > 200 {
			snippet = snippet[:197] + "..."
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("**Pending soul patch** `%s`:\n```\n%s\n```", sid, snippet)}, nil
	}
}

func handleGenericApprovalIntent(ctx context.Context, intent *ApprovalIntent, approvals *task.ApprovalStore, channelName string) (*cairnchannel.OutgoingMessage, error) {
	if approvals == nil {
		return &cairnchannel.OutgoingMessage{Text: "Approval store not configured."}, nil
	}

	// Specific ID provided — resolve prefix first.
	if intent.TargetID != "" {
		id, err := resolveApprovalID(ctx, approvals, intent.TargetID)
		if err != nil {
			return &cairnchannel.OutgoingMessage{Text: err.Error()}, nil
		}
		return applyApprovalAction(ctx, approvals, id, intent.Action, channelName)
	}

	// No ID — check pending.
	pending, err := approvals.ListPending(ctx)
	if err != nil {
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error: %s", err)}, nil
	}
	if len(pending) == 0 {
		return &cairnchannel.OutgoingMessage{Text: "No pending approvals."}, nil
	}
	if len(pending) == 1 {
		return applyApprovalAction(ctx, approvals, pending[0].ID, intent.Action, channelName)
	}
	// Multiple — list.
	var b strings.Builder
	fmt.Fprintf(&b, "**%d pending approvals** — which one?\n\n", len(pending))
	for _, a := range pending {
		snippet := a.Description
		if len(snippet) > 60 {
			snippet = snippet[:57] + "..."
		}
		fmt.Fprintf(&b, "`%s` [%s] %s\n", shortID(a.ID), a.Type, snippet)
	}
	b.WriteString("\nReply with the ID.")
	return &cairnchannel.OutgoingMessage{Text: b.String()}, nil
}

// resolveApprovalID resolves a short prefix to a full approval ID.
func resolveApprovalID(ctx context.Context, store *task.ApprovalStore, prefix string) (string, error) {
	// Try exact match first.
	if _, err := store.Get(ctx, prefix); err == nil {
		return prefix, nil
	}
	// Prefix search across pending approvals.
	pending, err := store.ListPending(ctx)
	if err != nil {
		return "", fmt.Errorf("error listing approvals: %w", err)
	}
	var matches []string
	for _, a := range pending {
		if strings.HasPrefix(a.ID, prefix) {
			matches = append(matches, a.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no approval found with ID or prefix `%s`", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous prefix `%s` — matches %d approvals", prefix, len(matches))
	}
}

func applyApprovalAction(ctx context.Context, approvals *task.ApprovalStore, id string, action ApprovalAction, channelName string) (*cairnchannel.OutgoingMessage, error) {
	sid := shortID(id)
	switch action {
	case ActionApprove:
		if err := approvals.Approve(ctx, id, channelName); err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error approving `%s`: %s", sid, err)}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Approval `%s` approved.", sid)}, nil
	case ActionDeny:
		if err := approvals.Deny(ctx, id, channelName); err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error denying `%s`: %s", sid, err)}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Approval `%s` denied.", sid)}, nil
	default:
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Approval `%s` is pending.", sid)}, nil
	}
}

// handleUnknownTarget resolves "yes"/"no" by checking all pending items.
func handleUnknownTarget(
	ctx context.Context,
	intent *ApprovalIntent,
	memSvc *memory.Service,
	soul *memory.Soul,
	approvals *task.ApprovalStore,
	channelName string,
) (*cairnchannel.OutgoingMessage, error) {
	items, err := gatherPending(ctx, memSvc, soul, approvals)
	if err != nil {
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error: %s", err)}, nil
	}

	if len(items) == 0 {
		return &cairnchannel.OutgoingMessage{Text: "Nothing pending to approve."}, nil
	}

	if len(items) == 1 {
		it := items[0]
		switch it.kind {
		case "memory":
			return applyMemoryAction(ctx, memSvc, it.id, intent.Action)
		case "soul_patch":
			return handleSoulPatchIntent(ctx, intent, soul)
		case "approval":
			return applyApprovalAction(ctx, approvals, it.id, intent.Action, channelName)
		}
	}

	// Multiple pending items — list them all.
	var b strings.Builder
	fmt.Fprintf(&b, "**%d pending items** — which one?\n\n", len(items))
	for i, it := range items {
		fmt.Fprintf(&b, "%d. %s\n", i+1, it.display)
	}
	b.WriteString("\nReply with an ID or be more specific (e.g. 'approve the memory', 'deny the soul patch').")
	return &cairnchannel.OutgoingMessage{Text: b.String()}, nil
}

// handleCallbackData processes button callback data like "approve:apr_abc123".
// channelName identifies where the callback came from.
func handleCallbackData(
	ctx context.Context,
	data string,
	memSvc *memory.Service,
	soul *memory.Soul,
	approvals *task.ApprovalStore,
	channelName string,
) (*cairnchannel.OutgoingMessage, error) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Invalid callback: %s", data)}, nil
	}

	actionStr, id := parts[0], parts[1]
	var action ApprovalAction
	switch actionStr {
	case "approve":
		action = ActionApprove
	case "deny":
		action = ActionDeny
	default:
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Unknown action: %s", actionStr)}, nil
	}

	// Try approval store first — distinguish not-found from real errors.
	if approvals != nil {
		if _, err := approvals.Get(ctx, id); err == nil {
			return applyApprovalAction(ctx, approvals, id, action, channelName)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error checking approval `%s`: %s", shortID(id), err)}, nil
		}
	}

	// Try memory by prefix — surface disambiguation errors.
	if memSvc != nil {
		resolved, err := resolveMemoryID(ctx, memSvc, id)
		if err == nil {
			return applyMemoryAction(ctx, memSvc, resolved, action)
		}
		// resolveMemoryID returns a descriptive error for "not found" and "ambiguous";
		// only fall through on "not found", surface other errors.
		if !strings.Contains(err.Error(), "no memory found") {
			return &cairnchannel.OutgoingMessage{Text: err.Error()}, nil
		}
	}

	// Try soul patch.
	if soul != nil {
		if p := soul.PendingPatch(); p != nil && strings.HasPrefix(p.ID, id) {
			return handleSoulPatchIntent(ctx, &ApprovalIntent{Action: action, Target: TargetSoulPatch}, soul)
		}
	}

	return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("No pending item found for `%s`.", shortID(id))}, nil
}
