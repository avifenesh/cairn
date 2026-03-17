package builtin

import (
	"github.com/avifenesh/cairn/internal/tool"
)

// All returns every built-in tool.
func All() []tool.Tool {
	return []tool.Tool{
		readFile,
		writeFile,
		editFile,
		deleteFile,
		listFiles,
		searchFiles,
		shell,
		gitRun,
	}
}
