package builtin

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

// --- Per-session file tracking (read-before-write, checkpoints) ---

const maxCheckpointsPerFile = 10

// fileCheckpoint stores a snapshot of a file before a mutating operation.
type fileCheckpoint struct {
	Content   []byte
	Timestamp time.Time
	Operation string // "write", "edit", "delete"
}

// fileSessionState holds per-session file tracking data.
type fileSessionState struct {
	mu          sync.Mutex
	readFiles   map[string]time.Time        // abs path -> last read time
	checkpoints map[string][]fileCheckpoint // abs path -> ring buffer
}

// sessions maps sessionID -> *fileSessionState.
var sessions sync.Map

func getOrCreateSessionState(sessionID string) *fileSessionState {
	if sessionID == "" {
		// MCP and other contexts may not have a session ID.
		// Use a per-goroutine fallback to avoid shared state.
		sessionID = "_anonymous"
	}
	if v, ok := sessions.Load(sessionID); ok {
		return v.(*fileSessionState)
	}
	st := &fileSessionState{
		readFiles:   make(map[string]time.Time),
		checkpoints: make(map[string][]fileCheckpoint),
	}
	actual, _ := sessions.LoadOrStore(sessionID, st)
	return actual.(*fileSessionState)
}

// CleanupSessionFiles removes per-session file tracking state.
func CleanupSessionFiles(sessionID string) {
	sessions.Delete(sessionID)
}

func (st *fileSessionState) recordRead(absPath string) {
	st.mu.Lock()
	st.readFiles[absPath] = time.Now()
	st.mu.Unlock()
}

func (st *fileSessionState) wasRead(absPath string) bool {
	st.mu.Lock()
	_, ok := st.readFiles[absPath]
	st.mu.Unlock()
	return ok
}

func (st *fileSessionState) snapshot(absPath, operation string) {
	data, err := os.ReadFile(absPath)
	if err != nil && !os.IsNotExist(err) {
		return // unreadable (permission error, etc.) — skip checkpoint
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	cp := fileCheckpoint{
		Timestamp: time.Now(),
		Operation: operation,
	}
	if err == nil {
		cp.Content = data
	}
	// os.IsNotExist → Content stays nil (sentinel: "delete to undo creation").
	cps := st.checkpoints[absPath]
	cps = append(cps, cp)
	if len(cps) > maxCheckpointsPerFile {
		cps = cps[len(cps)-maxCheckpointsPerFile:]
	}
	st.checkpoints[absPath] = cps
}

func (st *fileSessionState) popCheckpoint(absPath string) (fileCheckpoint, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	cps := st.checkpoints[absPath]
	if len(cps) == 0 {
		return fileCheckpoint{}, false
	}
	last := cps[len(cps)-1]
	st.checkpoints[absPath] = cps[:len(cps)-1]
	return last, true
}

func (st *fileSessionState) checkpointCount(absPath string) int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.checkpoints[absPath])
}

// checkReadBeforeWrite returns a warning string if the file exists but was
// not read in this session. Returns empty string if OK or file is new.
func checkReadBeforeWrite(ctx *tool.ToolContext, absPath string) string {
	st := getOrCreateSessionState(ctx.SessionID)
	if st.wasRead(absPath) {
		return ""
	}
	if _, err := os.Stat(absPath); err != nil {
		return "" // file doesn't exist yet — new file
	}
	return "[WARNING] Writing to file that was not read in this session. " +
		"Read the file first to avoid overwriting unintended content.\n"
}

// --- Fuzzy matching helpers ---

// fuzzyMatch tries whitespace-normalized line-by-line comparison when exact
// match fails. Returns the original matched text and byte offsets, or empty
// string if no unique match found.
func fuzzyMatch(content, old string) (matched string, start, end int) {
	contentLines := strings.Split(content, "\n")
	oldLines := strings.Split(old, "\n")

	oldNorm := trimEmptyStrings(normalizeLines(oldLines))
	if len(oldNorm) == 0 {
		return "", 0, 0
	}

	type match struct{ startLine, endLine int }
	var matches []match

	for i := 0; i <= len(contentLines)-len(oldNorm); i++ {
		allMatch := true
		for j, normLine := range oldNorm {
			if strings.TrimSpace(contentLines[i+j]) != normLine {
				allMatch = false
				break
			}
		}
		if allMatch {
			matches = append(matches, match{i, i + len(oldNorm)})
		}
	}

	if len(matches) != 1 {
		return "", 0, 0
	}

	m := matches[0]
	start = 0
	for i := 0; i < m.startLine; i++ {
		start += len(contentLines[i]) + 1
	}
	matchedLines := contentLines[m.startLine:m.endLine]
	matched = strings.Join(matchedLines, "\n")
	end = start + len(matched)
	return matched, start, end
}

func normalizeLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimSpace(l)
	}
	return out
}

func trimEmptyStrings(ss []string) []string {
	start, end := 0, len(ss)
	for start < end && ss[start] == "" {
		start++
	}
	for end > start && ss[end-1] == "" {
		end--
	}
	return ss[start:end]
}

// --- Better error diagnostics ---

// buildEditDiagnostic creates a helpful error when a search string is not found.
func buildEditDiagnostic(content, old string) string {
	oldLines := strings.Split(old, "\n")
	preview := oldLines
	if len(preview) > 3 {
		preview = preview[:3]
	}

	var sb strings.Builder
	sb.WriteString("Search text (first 3 lines):\n")
	for _, l := range preview {
		sb.WriteString("  > " + l + "\n")
	}

	firstLine := ""
	for _, l := range oldLines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			firstLine = trimmed
			break
		}
	}
	if firstLine == "" {
		return sb.String()
	}

	contentLines := strings.Split(content, "\n")
	bestLine := -1
	bestScore := 0
	for i, cl := range contentLines {
		trimmed := strings.TrimSpace(cl)
		if trimmed == "" {
			continue
		}
		score := commonPrefixLen(trimmed, firstLine)
		if score > bestScore {
			bestScore = score
			bestLine = i
		}
	}

	if bestLine >= 0 && bestScore > len(firstLine)/3 {
		start := bestLine - 1
		if start < 0 {
			start = 0
		}
		end := bestLine + 3
		if end > len(contentLines) {
			end = len(contentLines)
		}
		sb.WriteString(fmt.Sprintf("Closest match in file (line %d):\n", bestLine+1))
		for i := start; i < end; i++ {
			sb.WriteString(fmt.Sprintf("  %d: %s\n", i+1, contentLines[i]))
		}
	} else {
		sb.WriteString("No similar content found in file. Verify the file path and content are correct.\n")
	}
	return sb.String()
}

func commonPrefixLen(a, b string) int {
	max := len(a)
	if len(b) < max {
		max = len(b)
	}
	for i := 0; i < max; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return max
}

// truncate returns s truncated to maxLen with "..." appended if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// --- Tools ---

// readFileParams are the parameters for cairn.readFile.
type readFileParams struct {
	Path   string `json:"path" desc:"File path to read (relative to work directory)"`
	Offset *int   `json:"offset,omitempty" desc:"Line number to start reading from (1-based, default: 1)"`
	Limit  *int   `json:"limit,omitempty" desc:"Maximum number of lines to read (0 = unlimited)"`
}

var readFile = tool.Define("cairn.readFile",
	"Read the contents of a file. Supports offset (start line, 1-based) and limit (max lines) for large files.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p readFileParams) (*tool.ToolResult, error) {
		absPath, err := safePath(ctx.WorkDir, p.Path)
		if err != nil {
			return nil, err
		}

		if action := ctx.Permissions.Evaluate("cairn.readFile", absPath); action != tool.Allow {
			return &tool.ToolResult{Error: fmt.Sprintf("permission denied: read %s", p.Path)}, nil
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to read file: %v", err)}, nil
		}

		// Track this file as read for read-before-write enforcement.
		st := getOrCreateSessionState(ctx.SessionID)
		st.recordRead(absPath)

		lines := strings.Split(string(data), "\n")
		totalLines := len(lines)

		startLine := 0
		if p.Offset != nil && *p.Offset > 1 {
			startLine = *p.Offset - 1
			if startLine >= totalLines {
				startLine = totalLines
			}
		}

		endLine := totalLines
		if p.Limit != nil && *p.Limit > 0 {
			if startLine+*p.Limit < endLine {
				endLine = startLine + *p.Limit
			}
		}

		output := strings.Join(lines[startLine:endLine], "\n")

		// Clamp metadata so fromLine <= toLine always.
		fromLine := startLine + 1
		toLine := endLine
		if fromLine > toLine {
			fromLine = toLine
		}

		return &tool.ToolResult{
			Output: output,
			Metadata: map[string]any{
				"path":       absPath,
				"size":       len(data),
				"totalLines": totalLines,
				"fromLine":   fromLine,
				"toLine":     toLine,
			},
		}, nil
	},
)

// writeFileParams are the parameters for cairn.writeFile.
type writeFileParams struct {
	Path          string `json:"path" desc:"File path to write (relative to work directory)"`
	Content       string `json:"content" desc:"Content to write to the file"`
	ExpectedLines *int   `json:"expectedLines,omitempty" desc:"Expected line count. If provided and actual differs, a truncation warning is shown."`
}

var writeFile = tool.Define("cairn.writeFile",
	"Write content to a file. Creates parent directories if needed.",
	[]tool.Mode{tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p writeFileParams) (*tool.ToolResult, error) {
		absPath, err := safePath(ctx.WorkDir, p.Path)
		if err != nil {
			return nil, err
		}

		if action := ctx.Permissions.Evaluate("cairn.writeFile", absPath); action != tool.Allow {
			return &tool.ToolResult{Error: fmt.Sprintf("permission denied: write %s", p.Path)}, nil
		}

		// Read-before-write check.
		var warning string
		warning += checkReadBeforeWrite(ctx, absPath)

		// Checkpoint before overwriting.
		st := getOrCreateSessionState(ctx.SessionID)
		st.snapshot(absPath, "write")

		dir := filepath.Dir(absPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to create directory: %v", err)}, nil
		}

		if err := os.WriteFile(absPath, []byte(p.Content), 0644); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to write file: %v", err)}, nil
		}

		// Line count validation.
		if p.ExpectedLines != nil {
			actualLines := countLines(p.Content)
			if actualLines != *p.ExpectedLines {
				warning += fmt.Sprintf("[WARNING] Expected %d lines but content has %d lines. "+
					"This may indicate truncated output.\n", *p.ExpectedLines, actualLines)
			}
		}

		return &tool.ToolResult{
			Output: warning + fmt.Sprintf("Wrote %d bytes to %s", len(p.Content), absPath),
			Metadata: map[string]any{
				"path":  absPath,
				"bytes": len(p.Content),
			},
		}, nil
	},
)

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// editFileParams are the parameters for cairn.editFile.
type editFileParams struct {
	Path       string `json:"path" desc:"File path to edit (relative to work directory)"`
	Old        string `json:"old" desc:"Text to search for (exact match)"`
	New        string `json:"new" desc:"Replacement text"`
	ReplaceAll bool   `json:"replaceAll" desc:"Replace all occurrences (default: first only)"`
}

var editFile = tool.Define("cairn.editFile",
	"Edit a file by replacing text. Searches for the old string and replaces with the new string. "+
		"Errors if old matches multiple locations (use replaceAll or provide more context).",
	[]tool.Mode{tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p editFileParams) (*tool.ToolResult, error) {
		absPath, err := safePath(ctx.WorkDir, p.Path)
		if err != nil {
			return nil, err
		}

		if action := ctx.Permissions.Evaluate("cairn.editFile", absPath); action != tool.Allow {
			return &tool.ToolResult{Error: fmt.Sprintf("permission denied: edit %s", p.Path)}, nil
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to read file: %v", err)}, nil
		}

		content := string(data)

		// Check read-before-write BEFORE recording this as a read.
		st := getOrCreateSessionState(ctx.SessionID)
		var warning string
		if !st.wasRead(absPath) {
			warning += checkReadBeforeWrite(ctx, absPath)
		}
		// Mark as read after the check — editing loads the file content.
		st.recordRead(absPath)

		if !strings.Contains(content, p.Old) {
			// Try fuzzy match (whitespace-normalized).
			match, matchStart, matchEnd := fuzzyMatch(content, p.Old)
			if match != "" {
				// Checkpoint before fuzzy edit.
				st.snapshot(absPath, "edit")

				newContent := content[:matchStart] + p.New + content[matchEnd:]
				if err := os.WriteFile(absPath, []byte(newContent), 0644); err != nil {
					return &tool.ToolResult{Error: fmt.Sprintf("failed to write file: %v", err)}, nil
				}

				return &tool.ToolResult{
					Output: fmt.Sprintf("[WARN] Exact match not found. Applied whitespace-normalized match in %s.\nMatched: %q",
						absPath, truncate(match, 120)),
					Metadata: map[string]any{"path": absPath, "replacements": 1, "fuzzyMatch": true},
				}, nil
			}

			// No match at all — show diagnostic.
			diagnostic := buildEditDiagnostic(content, p.Old)
			return &tool.ToolResult{
				Error: fmt.Sprintf("search string not found in file.\n%s", diagnostic),
			}, nil
		}

		// Ambiguous match detection.
		if !p.ReplaceAll {
			count := strings.Count(content, p.Old)
			if count > 1 {
				locations := findMatchLocations(content, p.Old)
				return &tool.ToolResult{
					Error: fmt.Sprintf("ambiguous match: found %d occurrences of search string. "+
						"Provide more context in 'old' to match uniquely, or set replaceAll=true.\n"+
						"Occurrences:\n%s", count, strings.Join(locations, "\n")),
				}, nil
			}
		}

		// Checkpoint before edit.
		st.snapshot(absPath, "edit")

		var newContent string
		var count int
		if p.ReplaceAll {
			count = strings.Count(content, p.Old)
			newContent = strings.ReplaceAll(content, p.Old, p.New)
		} else {
			count = 1
			newContent = strings.Replace(content, p.Old, p.New, 1)
		}

		if err := os.WriteFile(absPath, []byte(newContent), 0644); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to write file: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output: warning + fmt.Sprintf("Replaced %d occurrence(s) in %s", count, absPath),
			Metadata: map[string]any{
				"path":         absPath,
				"replacements": count,
			},
		}, nil
	},
)

// findMatchLocations returns line-number descriptions for each occurrence of old in content.
func findMatchLocations(content, old string) []string {
	if old == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	// Build line start offsets for byte-offset to line-number mapping.
	lineStarts := make([]int, len(lines))
	offset := 0
	for i, line := range lines {
		lineStarts[i] = offset
		offset += len(line) + 1
	}

	var locations []string
	searchFrom := 0
	for {
		idx := strings.Index(content[searchFrom:], old)
		if idx == -1 {
			break
		}
		matchPos := searchFrom + idx
		lineNum := 0
		for i := len(lineStarts) - 1; i >= 0; i-- {
			if matchPos >= lineStarts[i] {
				lineNum = i + 1
				break
			}
		}
		if lineNum > 0 && lineNum <= len(lines) {
			preview := strings.TrimSpace(lines[lineNum-1])
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			locations = append(locations, fmt.Sprintf("  line %d: %s", lineNum, preview))
		}
		searchFrom = matchPos + len(old)
		if searchFrom >= len(content) {
			break
		}
	}
	return locations
}

// deleteFileParams are the parameters for cairn.deleteFile.
type deleteFileParams struct {
	Path string `json:"path" desc:"File path to delete (relative to work directory)"`
}

var deleteFile = tool.Define("cairn.deleteFile",
	"Delete a file.",
	[]tool.Mode{tool.ModeCoding},
	func(ctx *tool.ToolContext, p deleteFileParams) (*tool.ToolResult, error) {
		absPath, err := safePath(ctx.WorkDir, p.Path)
		if err != nil {
			return nil, err
		}

		if action := ctx.Permissions.Evaluate("cairn.deleteFile", absPath); action != tool.Allow {
			return &tool.ToolResult{Error: fmt.Sprintf("permission denied: delete %s", p.Path)}, nil
		}

		// Checkpoint before delete.
		st := getOrCreateSessionState(ctx.SessionID)
		st.snapshot(absPath, "delete")

		if err := os.Remove(absPath); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to delete file: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output: fmt.Sprintf("Deleted %s", absPath),
			Metadata: map[string]any{
				"path": absPath,
			},
		}, nil
	},
)

// undoEditParams are the parameters for cairn.undoEdit.
type undoEditParams struct {
	Path string `json:"path" desc:"File path to undo the last edit for (relative to work directory)"`
}

var undoEdit = tool.Define("cairn.undoEdit",
	"Undo the most recent edit/write/delete to a file, restoring its previous content from the session checkpoint.",
	[]tool.Mode{tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p undoEditParams) (*tool.ToolResult, error) {
		absPath, err := safePath(ctx.WorkDir, p.Path)
		if err != nil {
			return nil, err
		}

		if action := ctx.Permissions.Evaluate("cairn.undoEdit", absPath); action != tool.Allow {
			return &tool.ToolResult{Error: fmt.Sprintf("permission denied: undo %s", p.Path)}, nil
		}

		st := getOrCreateSessionState(ctx.SessionID)
		cp, ok := st.popCheckpoint(absPath)
		if !ok {
			return &tool.ToolResult{Error: "no checkpoints available for this file in this session"}, nil
		}

		if cp.Content == nil {
			// Sentinel: file didn't exist before — undo means delete.
			os.Remove(absPath) // best-effort; ignore errors if already gone
			return &tool.ToolResult{
				Output: fmt.Sprintf("Removed %s (file did not exist before %s)", absPath, cp.Operation),
				Metadata: map[string]any{
					"path":           absPath,
					"restoredBytes":  0,
					"remainingUndos": st.checkpointCount(absPath),
				},
			}, nil
		}

		dir := filepath.Dir(absPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to create directory: %v", err)}, nil
		}
		if err := os.WriteFile(absPath, cp.Content, 0644); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to restore file: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output: fmt.Sprintf("Restored %s to checkpoint from %s (before %s, %d bytes)",
				absPath, cp.Timestamp.Format("15:04:05"), cp.Operation, len(cp.Content)),
			Metadata: map[string]any{
				"path":           absPath,
				"restoredBytes":  len(cp.Content),
				"remainingUndos": st.checkpointCount(absPath),
			},
		}, nil
	},
)

// listFilesParams are the parameters for cairn.listFiles.
type listFilesParams struct {
	Path    string `json:"path" desc:"Directory path to list (relative to work directory)"`
	Pattern string `json:"pattern" desc:"Optional glob pattern to filter files (e.g., *.go)"`
}

var listFiles = tool.Define("cairn.listFiles",
	"List files in a directory, optionally filtered by glob pattern.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p listFilesParams) (*tool.ToolResult, error) {
		dir := p.Path
		if dir == "" {
			dir = "."
		}
		absPath, err := safePath(ctx.WorkDir, dir)
		if err != nil {
			return nil, err
		}

		entries, err := os.ReadDir(absPath)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to list directory: %v", err)}, nil
		}

		var lines []string
		for _, e := range entries {
			name := e.Name()
			if p.Pattern != "" {
				matched, _ := filepath.Match(p.Pattern, name)
				if !matched {
					continue
				}
			}
			suffix := ""
			if e.IsDir() {
				suffix = "/"
			}
			lines = append(lines, name+suffix)
		}

		return &tool.ToolResult{
			Output: strings.Join(lines, "\n"),
			Metadata: map[string]any{
				"path":  absPath,
				"count": len(lines),
			},
		}, nil
	},
)

// searchFilesParams are the parameters for cairn.searchFiles.
type searchFilesParams struct {
	Pattern    string `json:"pattern" desc:"Regex pattern to search for"`
	Path       string `json:"path" desc:"Directory to search in (relative to work directory)"`
	MaxResults int    `json:"maxResults" desc:"Maximum number of results to return (default: 50)"`
}

var searchFiles = tool.Define("cairn.searchFiles",
	"Search for a regex pattern in files (grep). Returns matching lines with file paths and line numbers.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p searchFilesParams) (*tool.ToolResult, error) {
		dir := p.Path
		if dir == "" {
			dir = "."
		}
		absPath, err := safePath(ctx.WorkDir, dir)
		if err != nil {
			return nil, err
		}

		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("invalid regex: %v", err)}, nil
		}

		maxResults := p.MaxResults
		if maxResults <= 0 {
			maxResults = 50
		}

		var results []string
		err = filepath.Walk(absPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil // skip errors
			}
			if info.IsDir() {
				// Skip hidden directories.
				if strings.HasPrefix(info.Name(), ".") && path != absPath {
					return filepath.SkipDir
				}
				return nil
			}
			if len(results) >= maxResults {
				return filepath.SkipAll
			}
			// Skip binary files (simple heuristic: skip large files, non-text extensions).
			if info.Size() > 1<<20 { // 1MB
				return nil
			}

			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer f.Close()

			relPath, _ := filepath.Rel(absPath, path)
			scanner := bufio.NewScanner(f)
			lineNo := 0
			for scanner.Scan() {
				lineNo++
				if len(results) >= maxResults {
					break
				}
				line := scanner.Text()
				if re.MatchString(line) {
					results = append(results, fmt.Sprintf("%s:%d:%s", relPath, lineNo, line))
				}
			}
			return nil
		})
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("search failed: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output: strings.Join(results, "\n"),
			Metadata: map[string]any{
				"matches": len(results),
			},
		}, nil
	},
)

// safePath resolves a relative path against the work directory and ensures the
// result does not escape the work directory. Resolves symlinks to prevent
// symlink-based directory escape attacks.
func safePath(workDir, rel string) (string, error) {
	if workDir == "" {
		return "", fmt.Errorf("work directory not set")
	}

	// Make workDir absolute and resolve symlinks.
	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("invalid work directory: %w", err)
	}
	realWork, err := filepath.EvalSymlinks(absWork)
	if err != nil {
		return "", fmt.Errorf("invalid work directory: %w", err)
	}

	var target string
	if filepath.IsAbs(rel) {
		target = filepath.Clean(rel)
	} else {
		target = filepath.Clean(filepath.Join(realWork, rel))
	}

	// Resolve symlinks on the target if it exists, to prevent symlink escapes.
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	} else {
		// Target doesn't exist yet (write/create) — resolve the nearest existing parent
		// to catch symlinked directories used in the path.
		parent := filepath.Dir(target)
		if resolvedParent, err := filepath.EvalSymlinks(parent); err == nil {
			target = filepath.Join(resolvedParent, filepath.Base(target))
		}
	}

	// Ensure the resolved path is within the work directory.
	if !strings.HasPrefix(target, realWork+string(filepath.Separator)) && target != realWork {
		return "", fmt.Errorf("path traversal denied: %s resolves outside work directory %s", rel, realWork)
	}

	return target, nil
}
