package builtin

import (
	"github.com/avifenesh/cairn/internal/tool"
)

// All returns every built-in tool.
func All() []tool.Tool {
	return []tool.Tool{
		// Filesystem tools.
		readFile,
		writeFile,
		editFile,
		deleteFile,
		listFiles,
		searchFiles,
		shell,
		gitRun,
		// Memory tools.
		createMemory,
		searchMemory,
		manageMemory,
		// Feed tools.
		readFeed,
		markRead,
		digest,
		// Journal tool.
		journalSearch,
		// Web tools.
		webSearch,
		webFetch,
	}
}
