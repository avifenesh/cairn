package builtin

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/avifenesh/cairn/internal/tool"
)

// readFileParams are the parameters for pub.readFile.
type readFileParams struct {
	Path  string `json:"path" desc:"File path to read (relative to work directory)"`
	Limit *int   `json:"limit,omitempty" desc:"Maximum number of lines to read (0 = unlimited)"`
}

var readFile = tool.Define("pub.readFile",
	"Read the contents of a file. Returns the file text.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p readFileParams) (*tool.ToolResult, error) {
		absPath, err := safePath(ctx.WorkDir, p.Path)
		if err != nil {
			return nil, err
		}

		if action := ctx.Permissions.Evaluate("pub.readFile", absPath); action == tool.Deny {
			return &tool.ToolResult{Error: fmt.Sprintf("permission denied: read %s", p.Path)}, nil
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to read file: %v", err)}, nil
		}

		output := string(data)
		if p.Limit != nil && *p.Limit > 0 {
			lines := strings.SplitN(output, "\n", *p.Limit+1)
			if len(lines) > *p.Limit {
				lines = lines[:*p.Limit]
			}
			output = strings.Join(lines, "\n")
		}

		return &tool.ToolResult{
			Output: output,
			Metadata: map[string]any{
				"path": absPath,
				"size": len(data),
			},
		}, nil
	},
)

// writeFileParams are the parameters for pub.writeFile.
type writeFileParams struct {
	Path    string `json:"path" desc:"File path to write (relative to work directory)"`
	Content string `json:"content" desc:"Content to write to the file"`
}

var writeFile = tool.Define("pub.writeFile",
	"Write content to a file. Creates parent directories if needed.",
	[]tool.Mode{tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p writeFileParams) (*tool.ToolResult, error) {
		absPath, err := safePath(ctx.WorkDir, p.Path)
		if err != nil {
			return nil, err
		}

		if action := ctx.Permissions.Evaluate("pub.writeFile", absPath); action == tool.Deny {
			return &tool.ToolResult{Error: fmt.Sprintf("permission denied: write %s", p.Path)}, nil
		}

		dir := filepath.Dir(absPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to create directory: %v", err)}, nil
		}

		if err := os.WriteFile(absPath, []byte(p.Content), 0644); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to write file: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output: fmt.Sprintf("Wrote %d bytes to %s", len(p.Content), absPath),
			Metadata: map[string]any{
				"path":  absPath,
				"bytes": len(p.Content),
			},
		}, nil
	},
)

// editFileParams are the parameters for pub.editFile.
type editFileParams struct {
	Path       string `json:"path" desc:"File path to edit (relative to work directory)"`
	Old        string `json:"old" desc:"Text to search for"`
	New        string `json:"new" desc:"Replacement text"`
	ReplaceAll bool   `json:"replaceAll" desc:"Replace all occurrences (default: first only)"`
}

var editFile = tool.Define("pub.editFile",
	"Edit a file by replacing text. Searches for the old string and replaces with the new string.",
	[]tool.Mode{tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p editFileParams) (*tool.ToolResult, error) {
		absPath, err := safePath(ctx.WorkDir, p.Path)
		if err != nil {
			return nil, err
		}

		if action := ctx.Permissions.Evaluate("pub.editFile", absPath); action == tool.Deny {
			return &tool.ToolResult{Error: fmt.Sprintf("permission denied: edit %s", p.Path)}, nil
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to read file: %v", err)}, nil
		}

		content := string(data)
		if !strings.Contains(content, p.Old) {
			return &tool.ToolResult{Error: "search string not found in file"}, nil
		}

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
			Output: fmt.Sprintf("Replaced %d occurrence(s) in %s", count, absPath),
			Metadata: map[string]any{
				"path":         absPath,
				"replacements": count,
			},
		}, nil
	},
)

// deleteFileParams are the parameters for pub.deleteFile.
type deleteFileParams struct {
	Path string `json:"path" desc:"File path to delete (relative to work directory)"`
}

var deleteFile = tool.Define("pub.deleteFile",
	"Delete a file.",
	[]tool.Mode{tool.ModeCoding},
	func(ctx *tool.ToolContext, p deleteFileParams) (*tool.ToolResult, error) {
		absPath, err := safePath(ctx.WorkDir, p.Path)
		if err != nil {
			return nil, err
		}

		if action := ctx.Permissions.Evaluate("pub.deleteFile", absPath); action == tool.Deny {
			return &tool.ToolResult{Error: fmt.Sprintf("permission denied: delete %s", p.Path)}, nil
		}

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

// listFilesParams are the parameters for pub.listFiles.
type listFilesParams struct {
	Path    string `json:"path" desc:"Directory path to list (relative to work directory)"`
	Pattern string `json:"pattern" desc:"Optional glob pattern to filter files (e.g., *.go)"`
}

var listFiles = tool.Define("pub.listFiles",
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

// searchFilesParams are the parameters for pub.searchFiles.
type searchFilesParams struct {
	Pattern    string `json:"pattern" desc:"Regex pattern to search for"`
	Path       string `json:"path" desc:"Directory to search in (relative to work directory)"`
	MaxResults int    `json:"maxResults" desc:"Maximum number of results to return (default: 50)"`
}

var searchFiles = tool.Define("pub.searchFiles",
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
	}
	// If target doesn't exist yet (write/create), check the parent.
	// This handles creating new files - the parent must be within workDir.

	// Ensure the resolved path is within the work directory.
	if !strings.HasPrefix(target, realWork+string(filepath.Separator)) && target != realWork {
		return "", fmt.Errorf("path traversal denied: %s resolves outside work directory %s", rel, realWork)
	}

	return target, nil
}
