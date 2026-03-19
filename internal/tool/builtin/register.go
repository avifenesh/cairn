package builtin

import (
	"github.com/avifenesh/cairn/internal/tool"
)

// displayTimeFormat is the human-readable time format used in tool output.
const displayTimeFormat = "2006-01-02 15:04"

// All returns every built-in tool. When Z.ai is configured (GLM provider),
// webSearch and webFetch use Z.ai APIs; otherwise they use SearXNG/direct fetch.
// Z.ai-only tools (searchDoc, repoStructure, readRepoFile) are added when Z.ai is enabled.
func All() []tool.Tool {
	tools := []tool.Tool{
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

	// Web tools: Z.ai (GLM) or SearXNG/direct.
	if ZaiEnabled() {
		tools = append(tools,
			zaiWebSearch,     // cairn.webSearch backed by Z.ai
			zaiWebReader,     // cairn.webFetch backed by Z.ai
			zaiSearchDoc,     // cairn.searchDoc (Z.ai only)
			zaiRepoStructure, // cairn.repoStructure (Z.ai only)
			zaiReadRepoFile,  // cairn.readRepoFile (Z.ai only)
		)
		if VisionEnabled() {
			tools = append(tools,
				visionImageAnalysis,  // cairn.imageAnalysis
				visionExtractText,    // cairn.extractText
				visionDiagnoseError,  // cairn.diagnoseError
				visionAnalyzeDiagram, // cairn.analyzeDiagram
				visionAnalyzeChart,   // cairn.analyzeChart
				visionUIToArtifact,   // cairn.uiToArtifact
				visionUIDiffCheck,    // cairn.uiDiffCheck
				visionVideoAnalysis,  // cairn.videoAnalysis
			)
		}
	} else {
		tools = append(tools,
			webSearch, // cairn.webSearch backed by SearXNG
			webFetch,  // cairn.webFetch backed by direct HTTP
		)
	}

	return tools
}
