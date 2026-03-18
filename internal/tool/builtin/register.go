package builtin

import (
	"github.com/avifenesh/cairn/internal/tool"
)

// displayTimeFormat is the human-readable time format used in tool output.
const displayTimeFormat = "2006-01-02 15:04"

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
		// Task tools.
		createTask,
		listTasks,
		completeTask,
		// Communication + status tools.
		compose,
		getStatus,
		// Skill tools.
		loadSkill,
		listSkills,
	}
}
