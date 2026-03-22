package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Config holds all application configuration, loaded from environment variables.
type Config struct {
	// Server
	Port int
	Host string

	// Database
	DatabasePath string

	// LLM — provider-agnostic
	LLMProvider      string // "glm", "openai"
	LLMAPIKey        string
	LLMBaseURL       string
	LLMModel         string
	LLMFallbackModel string

	// Legacy GLM aliases (read if LLM_* not set)
	GLMAPIKey  string
	GLMBaseURL string
	GLMModel   string

	// Auth tokens
	WriteAPIToken string
	ReadAPIToken  string

	// Frontend
	FrontendOrigin string

	// Feature flags
	CodingEnabled   bool
	IdleModeEnabled bool
	RulesEnabled    bool

	// Signal plane
	GHToken            string            // GitHub personal access token
	GHOrgs             []string          // GitHub orgs to track
	GHOwner            string            // Your GitHub login (for self-filter)
	GHTrackedRepos     []string          // Explicit repos to track (empty = auto-detect)
	GHBotFilter        []string          // Additional bot logins to filter
	GHMetricsInterval  int               // Seconds between metrics polls (default 14400 = 4h)
	GmailEnabled       bool              // Enable Gmail poller
	CalendarEnabled    bool              // Enable Calendar poller
	GmailFilterQuery   string            // Gmail search query filter
	CalendarLookaheadH int               // Calendar lookahead hours (default 48)
	RSSEnabled         bool              // Enable RSS poller
	RSSFeeds           []string          // RSS/Atom feed URLs
	SOEnabled          bool              // Enable Stack Overflow poller
	SOTags             []string          // SO tags to monitor
	SOAPIKey           string            // SO API key (optional, higher rate limit)
	SOPollInterval     int               // SO poll interval in minutes (default 60)
	DevToEnabled       bool              // Enable Dev.to poller
	DevToTags          []string          // Dev.to tags to monitor
	DevToUsername      string            // Dev.to username
	DevToPollInterval  int               // Dev.to poll interval in minutes (default 30)
	HNKeywords         []string          // HN keyword filter
	HNMinScore         int               // HN minimum score filter
	PollInterval       int               // Poll interval in seconds (default 300 = 5min)
	RedditSubs         []string          // Subreddits to monitor
	NPMPackages        []string          // npm packages to track
	CratesPackages     []string          // crates.io crates to track
	WebhookSecrets     map[string]string // webhook name -> HMAC secret

	// Memory context builder
	MemoryContextBudget   int     // Token budget (default: 4000)
	MemoryHardRuleReserve int     // Reserved for hard rules (default: 500)
	MemoryDecayHalfLife   float64 // Days (default: 30)
	MemoryStaleThreshold  float64 // Days (default: 14)

	// Embeddings
	EmbeddingEnabled    bool   // EMBEDDING_ENABLED (default: true when API key present)
	EmbeddingModel      string // EMBEDDING_MODEL (auto: "embedding-3" for GLM)
	EmbeddingDimensions int    // EMBEDDING_DIMENSIONS (default: 2048)
	EmbeddingBaseURL    string // EMBEDDING_BASE_URL (defaults to LLM base URL)
	EmbeddingAPIKey     string // EMBEDDING_API_KEY (defaults to LLM API key)

	// Budget
	BudgetDailyCap  float64 // Daily LLM spend cap USD (0 = unlimited)
	BudgetWeeklyCap float64 // Weekly LLM spend cap USD (0 = unlimited)

	// Agent loop
	AgentTickInterval  int // Seconds (default: 60)
	ReflectionInterval int // Seconds (default: 1800)

	// Tool round limits per mode
	TalkMaxRounds   int // TALK_MAX_ROUNDS (default: 40)
	WorkMaxRounds   int // WORK_MAX_ROUNDS (default: 80)
	CodingMaxRounds int // CODING_MAX_ROUNDS (default: 400)

	// Coding allowed repos — CSV of absolute repo paths where agent can create worktrees.
	// Empty = only the default repo (cwd), no restriction. When set, the default repo
	// must be included explicitly if coding should be allowed there.
	// Paths are normalized to absolute+clean on load.
	CodingAllowedRepos []string // CODING_ALLOWED_REPOS (comma-separated)

	// Memory auto-extraction
	MemoryAutoExtract bool // MEMORY_AUTO_EXTRACT (default: true)

	// Session compaction
	CompactionTriggerTokens int // COMPACTION_TRIGGER_TOKENS (default: 150000)
	CompactionKeepRecent    int // COMPACTION_KEEP_RECENT (default: 10)
	CompactionMaxToolOutput int // COMPACTION_MAX_TOOL_OUTPUT (default: 32000)

	// MCP server
	MCPServerEnabled  bool   // MCP_SERVER_ENABLED (default false)
	MCPPort           int    // MCP_PORT (default 3001)
	MCPTransport      string // MCP_TRANSPORT ("stdio"/"http"/"both", default "http")
	MCPWriteRateLimit int    // MCP_WRITE_RATE_LIMIT (default 100 per minute)

	// MCP client connections
	MCPClientServers json.RawMessage // MCP_SERVERS JSON array of server configs

	// Z.ai MCP tools (default for GLM provider)
	ZaiWebEnabled    bool   // ZAI_WEB_ENABLED (default true when LLM_PROVIDER=glm)
	ZaiBaseURL       string // ZAI_BASE_URL (default https://api.z.ai/api/mcp)
	ZaiAPIKey        string // ZAI_API_KEY (separate MCP key, falls back to LLM_API_KEY)
	ZaiVisionEnabled bool   // ZAI_VISION_ENABLED (default true when LLM_PROVIDER=glm)

	// Web tools (fallback when Z.ai disabled)
	SearXNGURL      string // SearXNG instance URL for web search
	WebFetchTimeout int    // Seconds (default: 30)
	WebFetchMaxSize int64  // Bytes (default: 5MB)

	// Channels — Telegram
	TelegramBotToken      string // TELEGRAM_BOT_TOKEN
	TelegramChatID        int64  // TELEGRAM_CHAT_ID
	ChannelSessionTimeout int    // CHANNEL_SESSION_TIMEOUT (minutes, default 240)

	// Channels — Discord
	DiscordBotToken  string // DISCORD_BOT_TOKEN
	DiscordChannelID string // DISCORD_CHANNEL_ID

	// Channels — Slack
	SlackBotToken  string // SLACK_BOT_TOKEN
	SlackAppToken  string // SLACK_APP_TOKEN (Socket Mode)
	SlackChannelID string // SLACK_CHANNEL_ID

	// Notification routing
	PreferredChannel string   // PREFERRED_CHANNEL (e.g. "telegram")
	QuietHoursStart  int      // QUIET_HOURS_START (0-23, -1 = disabled)
	QuietHoursEnd    int      // QUIET_HOURS_END (0-23, -1 = disabled)
	QuietHoursTZ     string   // QUIET_HOURS_TZ (IANA timezone, default "UTC")
	MutedSources     []string // MUTED_SOURCES — sources that don't generate notifications
	NotifMinPriority string   // NOTIF_MIN_PRIORITY — "low", "medium", "high" (default "low" = all)
	ChannelRouting   string   // CHANNEL_ROUTING — JSON: {"github_signal":"telegram","gmail":"slack"}

	// Voice
	VoiceEnabled bool   // VOICE_ENABLED (default: false)
	WhisperURL   string // WHISPER_URL (default: http://127.0.0.1:8178)
	TTSVoice     string // TTS_VOICE (default: en-US-BrianNeural)

	// Paths
	SoulPath  string
	SkillDirs []string
	DataDir   string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	// Read provider-agnostic vars first, fall back to GLM/Zhipu/OpenAI aliases.
	apiKey := envStr("LLM_API_KEY", envStr("GLM_API_KEY", envStr("ZHIPU_API_KEY", envStr("OPENAI_API_KEY", ""))))
	baseURL := envStr("LLM_BASE_URL", envStr("GLM_BASE_URL", envStr("ZHIPU_BASE_URL", envStr("OPENAI_BASE_URL", ""))))
	model := envStr("LLM_MODEL", envStr("GLM_MODEL", ""))
	provider := envStr("LLM_PROVIDER", "")

	// Auto-detect provider from env vars if not explicitly set.
	if provider == "" {
		switch {
		case envStr("GLM_API_KEY", envStr("ZHIPU_API_KEY", "")) != "":
			provider = "glm"
		case envStr("OPENAI_API_KEY", "") != "":
			provider = "openai"
		default:
			provider = "glm"
		}
	}
	// Normalize "zhipu" → "glm" (Pub v1 uses GLM_PROVIDER=zhipu).
	if provider == "zhipu" {
		provider = "glm"
	}

	// Apply provider-specific defaults.
	if baseURL == "" {
		switch provider {
		case "glm", "zhipu":
			baseURL = "https://api.z.ai/api/coding/paas/v4"
		case "openai":
			baseURL = "https://api.openai.com/v1"
		}
	}
	if model == "" {
		switch provider {
		case "glm", "zhipu":
			model = "glm-5-turbo"
		case "openai":
			model = "gpt-4o"
		}
	}

	c := &Config{
		Port:                    envInt("PORT", 8787),
		Host:                    envStr("HOST", "0.0.0.0"),
		DatabasePath:            envStr("DATABASE_PATH", "./data/cairn.db"),
		LLMProvider:             provider,
		LLMAPIKey:               apiKey,
		LLMBaseURL:              baseURL,
		LLMModel:                model,
		LLMFallbackModel:        envStr("LLM_FALLBACK_MODEL", envStr("GLM_FALLBACK_MODEL", "")),
		GLMAPIKey:               apiKey,
		GLMBaseURL:              baseURL,
		GLMModel:                model,
		WriteAPIToken:           envStr("WRITE_API_TOKEN", ""),
		ReadAPIToken:            envStr("READ_API_TOKEN", ""),
		FrontendOrigin:          envStr("FRONTEND_ORIGIN", ""),
		CodingEnabled:           envBool("CODING_ENABLED", false),
		IdleModeEnabled:         envBool("IDLE_MODE_ENABLED", false),
		RulesEnabled:            envBool("RULES_ENABLED", false),
		GHToken:                 envStr("GH_TOKEN", envStr("GITHUB_TOKEN", "")),
		GHOrgs:                  envSlice("GH_ORGS", nil),
		GHOwner:                 envStr("GH_OWNER", ""),
		GHTrackedRepos:          envSlice("GH_TRACKED_REPOS", nil),
		GHBotFilter:             envSlice("GH_BOT_FILTER", nil),
		GHMetricsInterval:       envInt("GH_METRICS_INTERVAL", 14400),
		GmailEnabled:            envBool("GMAIL_ENABLED", false),
		CalendarEnabled:         envBool("CALENDAR_ENABLED", false),
		GmailFilterQuery:        envStr("GMAIL_FILTER_QUERY", "-category:promotions -category:social -category:forums"),
		CalendarLookaheadH:      envInt("CALENDAR_LOOKAHEAD_H", 48),
		RSSEnabled:              envBool("RSS_ENABLED", false),
		RSSFeeds:                envSlice("RSS_FEEDS", nil),
		SOEnabled:               envBool("SO_ENABLED", false),
		SOTags:                  envSlice("SO_TAGS", nil),
		SOAPIKey:                envStr("SO_API_KEY", ""),
		SOPollInterval:          envInt("SO_POLL_INTERVAL", 60),
		DevToEnabled:            envBool("DEVTO_ENABLED", false),
		DevToTags:               envSlice("DEVTO_TAGS", nil),
		DevToUsername:           envStr("DEVTO_USERNAME", ""),
		DevToPollInterval:       envInt("DEVTO_POLL_INTERVAL", 30),
		HNKeywords:              envSlice("HN_KEYWORDS", nil),
		HNMinScore:              envInt("HN_MIN_SCORE", 0),
		PollInterval:            pollIntervalSeconds(),
		RedditSubs:              envSlice("REDDIT_SUBS", nil),
		NPMPackages:             envSlice("NPM_PACKAGES", nil),
		CratesPackages:          envSlice("CRATES_PACKAGES", envSlice("CRATES", nil)),
		WebhookSecrets:          envMap("WEBHOOK_SECRETS"),
		MemoryContextBudget:     envInt("MEMORY_CONTEXT_BUDGET", 4000),
		MemoryHardRuleReserve:   envInt("MEMORY_HARD_RULE_RESERVE", 500),
		MemoryDecayHalfLife:     envFloat("MEMORY_DECAY_HALF_LIFE", 30),
		MemoryStaleThreshold:    envFloat("MEMORY_STALE_THRESHOLD", 14),
		EmbeddingEnabled:        envBool("EMBEDDING_ENABLED", apiKey != ""),
		EmbeddingModel:          embeddingModel(provider),
		EmbeddingDimensions:     envInt("EMBEDDING_DIMENSIONS", 2048),
		EmbeddingBaseURL:        envStr("EMBEDDING_BASE_URL", baseURL),
		EmbeddingAPIKey:         envStr("EMBEDDING_API_KEY", apiKey),
		BudgetDailyCap:          envFloat("BUDGET_DAILY_CAP", envFloat("BEDROCK_DAILY_BUDGET_USD", envFloat("IDLE_BUDGET_CAP_USD", 0))),
		BudgetWeeklyCap:         envFloat("BUDGET_WEEKLY_CAP", envFloat("BEDROCK_WEEKLY_BUDGET_USD", 0)),
		AgentTickInterval:       envInt("AGENT_TICK_INTERVAL", 60),
		ReflectionInterval:      envInt("REFLECTION_INTERVAL", 1800),
		TalkMaxRounds:           envInt("TALK_MAX_ROUNDS", 40),
		WorkMaxRounds:           envInt("WORK_MAX_ROUNDS", 80),
		CodingMaxRounds:         envInt("CODING_MAX_ROUNDS", 400),
		CodingAllowedRepos:      envCSV("CODING_ALLOWED_REPOS"),
		MemoryAutoExtract:       envBool("MEMORY_AUTO_EXTRACT", true),
		CompactionTriggerTokens: envInt("COMPACTION_TRIGGER_TOKENS", 150000),
		CompactionKeepRecent:    envInt("COMPACTION_KEEP_RECENT", 10),
		CompactionMaxToolOutput: envInt("COMPACTION_MAX_TOOL_OUTPUT", 32000),
		MCPServerEnabled:        envBool("MCP_SERVER_ENABLED", false),
		MCPPort:                 envInt("MCP_PORT", 3001),
		MCPTransport:            envStr("MCP_TRANSPORT", "http"),
		MCPWriteRateLimit:       envInt("MCP_WRITE_RATE_LIMIT", 100),
		MCPClientServers:        envJSON("MCP_SERVERS"),
		ZaiWebEnabled:           envBool("ZAI_WEB_ENABLED", provider == "glm"),
		ZaiBaseURL:              envStr("ZAI_BASE_URL", "https://api.z.ai/api/mcp"),
		ZaiAPIKey:               envStr("ZAI_API_KEY", ""),
		ZaiVisionEnabled:        envBool("ZAI_VISION_ENABLED", provider == "glm"),
		TelegramBotToken:        envStr("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:          envInt64("TELEGRAM_CHAT_ID", 0),
		ChannelSessionTimeout:   envInt("CHANNEL_SESSION_TIMEOUT", 240),
		DiscordBotToken:         envStr("DISCORD_BOT_TOKEN", ""),
		DiscordChannelID:        envStr("DISCORD_CHANNEL_ID", ""),
		SlackBotToken:           envStr("SLACK_BOT_TOKEN", ""),
		SlackAppToken:           envStr("SLACK_APP_TOKEN", ""),
		SlackChannelID:          envStr("SLACK_CHANNEL_ID", ""),
		PreferredChannel:        envStr("PREFERRED_CHANNEL", ""),
		QuietHoursStart:         envInt("QUIET_HOURS_START", -1),
		QuietHoursEnd:           envInt("QUIET_HOURS_END", -1),
		QuietHoursTZ:            envStr("QUIET_HOURS_TZ", "UTC"),
		MutedSources:            envSlice("MUTED_SOURCES", nil),
		NotifMinPriority:        envStr("NOTIF_MIN_PRIORITY", "low"),
		ChannelRouting:          envStr("CHANNEL_ROUTING", ""),
		SearXNGURL:              envStr("SEARXNG_URL", ""),
		WebFetchTimeout:         envInt("WEB_FETCH_TIMEOUT", 30),
		WebFetchMaxSize:         envInt64("WEB_FETCH_MAX_SIZE", 5*1024*1024),
		VoiceEnabled:            envBool("VOICE_ENABLED", false),
		WhisperURL:              envStr("WHISPER_URL", "http://127.0.0.1:8178"),
		TTSVoice:                envStr("TTS_VOICE", "en-US-BrianNeural"),
		SoulPath:                envStr("SOUL_PATH", "./SOUL.md"),
		SkillDirs:               skillDirs(),
		DataDir:                 envStr("DATA_DIR", "./data"),
	}

	if c.LLMAPIKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY (or GLM_API_KEY / OPENAI_API_KEY) is required")
	}

	return c, nil
}

// LoadOptional is like Load but does not error on missing API keys.
// Useful for testing or when LLM is not needed.
func LoadOptional() *Config {
	c, err := Load()
	if err != nil {
		// Return a config with whatever we could read, just no LLM key.
		c = &Config{
			Port:              envInt("PORT", 8787),
			Host:              envStr("HOST", "0.0.0.0"),
			DatabasePath:      envStr("DATABASE_PATH", "./data/cairn.db"),
			LLMProvider:       envStr("LLM_PROVIDER", "glm"),
			MCPServerEnabled:  envBool("MCP_SERVER_ENABLED", false),
			MCPPort:           envInt("MCP_PORT", 3001),
			MCPTransport:      envStr("MCP_TRANSPORT", "http"),
			MCPWriteRateLimit: envInt("MCP_WRITE_RATE_LIMIT", 100),
			MCPClientServers:  envJSON("MCP_SERVERS"),
			SoulPath:          envStr("SOUL_PATH", "./SOUL.md"),
			SkillDirs:         skillDirs(),
			DataDir:           envStr("DATA_DIR", "./data"),
		}
	}
	return c
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return fallback
}

func envSlice(key string, fallback []string) []string {
	if v := os.Getenv(key); v != "" {
		return strings.Split(v, ",")
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func envCSV(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Normalize to absolute canonical path for security.
		if abs, err := filepath.Abs(p); err == nil {
			p = filepath.Clean(abs)
		}
		result = append(result, p)
	}
	return result
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

// pollIntervalSeconds reads POLL_INTERVAL (seconds) or POLL_INTERVAL_MS (ms, Pub v1 compat).
func pollIntervalSeconds() int {
	if v := envInt("POLL_INTERVAL", 0); v > 0 {
		return v
	}
	if v := envInt("POLL_INTERVAL_MS", 0); v > 0 {
		return v / 1000
	}
	return 300 // default: 5 minutes
}

// skillDirs builds the default skill search path from well-known locations
// plus any extra directories from the SKILL_DIRS env var.
// Order: ["./skills", "~/.cairn/skills", ".cairn/skills"] + SKILL_DIRS extras.
// Later directories override earlier ones (Service.Discover uses last-wins by map key).
func skillDirs() []string {
	dirs := []string{"./skills"}

	// Expand ~ for home-based path.
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".cairn", "skills"))
	}

	dirs = append(dirs, ".cairn/skills")

	// agentskills.io cross-client compatibility path.
	dirs = append(dirs, ".agents/skills")

	// Append any extra directories from SKILL_DIRS env var, filtering empty entries.
	if extra := envSlice("SKILL_DIRS", nil); len(extra) > 0 {
		for _, d := range extra {
			d = strings.TrimSpace(d)
			if d != "" {
				dirs = append(dirs, d)
			}
		}
	}

	return dirs
}

// embeddingModel returns the default embedding model for the given LLM provider.
func embeddingModel(provider string) string {
	if v := envStr("EMBEDDING_MODEL", ""); v != "" {
		return v
	}
	switch provider {
	case "glm", "zhipu":
		return "embedding-3"
	case "openai":
		return "text-embedding-3-small"
	default:
		return "embedding-3"
	}
}

// envMap parses JSON from an env var into map[string]string.
// Example: WEBHOOK_SECRETS='{"github":"abc123","stripe":"xyz789"}'
func envMap(key string) map[string]string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(v), &m); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s contains invalid JSON, ignoring: %v\n", key, err)
		return nil
	}
	return m
}

func envJSON(key string) json.RawMessage {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	if !json.Valid([]byte(v)) {
		fmt.Fprintf(os.Stderr, "warning: %s contains invalid JSON, ignoring\n", key)
		return nil
	}
	return json.RawMessage(v)
}

// --- Runtime-editable config ---

// PatchableConfig holds fields that can be changed at runtime via PATCH /v1/config.
// All fields are pointers — nil means "don't change".
type PatchableConfig struct {
	CompactionTriggerTokens *int     `json:"compactionTriggerTokens,omitempty"`
	CompactionKeepRecent    *int     `json:"compactionKeepRecent,omitempty"`
	CompactionMaxToolOutput *int     `json:"compactionMaxToolOutput,omitempty"`
	BudgetDailyCap          *float64 `json:"budgetDailyCap,omitempty"`
	BudgetWeeklyCap         *float64 `json:"budgetWeeklyCap,omitempty"`
	ChannelSessionTimeout   *int     `json:"channelSessionTimeout,omitempty"`
	GHOwner                 *string  `json:"ghOwner,omitempty"`
	GHTrackedRepos          *string  `json:"ghTrackedRepos,omitempty"`    // comma-separated
	GHBotFilter             *string  `json:"ghBotFilter,omitempty"`       // comma-separated
	GHMetricsInterval       *int     `json:"ghMetricsInterval,omitempty"` // seconds
	GmailEnabled            *bool    `json:"gmailEnabled,omitempty"`
	CalendarEnabled         *bool    `json:"calendarEnabled,omitempty"`
	GmailFilterQuery        *string  `json:"gmailFilterQuery,omitempty"`
	CalendarLookaheadH      *int     `json:"calendarLookaheadH,omitempty"`
	PreferredChannel        *string  `json:"preferredChannel,omitempty"`
	QuietHoursStart         *int     `json:"quietHoursStart,omitempty"`
	QuietHoursEnd           *int     `json:"quietHoursEnd,omitempty"`
	QuietHoursTZ            *string  `json:"quietHoursTZ,omitempty"`
	MutedSources            *string  `json:"mutedSources,omitempty"`     // comma-sep source names
	NotifMinPriority        *string  `json:"notifMinPriority,omitempty"` // low, medium, high
	ChannelRouting          *string  `json:"channelRouting,omitempty"`   // JSON: {"source":"channel"}
	RSSEnabled              *bool    `json:"rssEnabled,omitempty"`
	RSSFeeds                *string  `json:"rssFeeds,omitempty"` // comma-sep URLs
	SOEnabled               *bool    `json:"soEnabled,omitempty"`
	SOTags                  *string  `json:"soTags,omitempty"` // comma-sep
	DevToEnabled            *bool    `json:"devtoEnabled,omitempty"`
	DevToTags               *string  `json:"devtoTags,omitempty"` // comma-sep
	DevToUsername           *string  `json:"devtoUsername,omitempty"`
	NPMPackages             *string  `json:"npmPackages,omitempty"`      // comma-sep
	CratesPackages          *string  `json:"cratesPackages,omitempty"`   // comma-sep
	MCPClientServers        *string  `json:"mcpClientServers,omitempty"` // JSON array of server configs
}

var configMu sync.RWMutex

// ApplyPatch merges non-nil fields from p into the config.
func (c *Config) ApplyPatch(p PatchableConfig) {
	configMu.Lock()
	defer configMu.Unlock()
	if p.CompactionTriggerTokens != nil && *p.CompactionTriggerTokens > 0 {
		c.CompactionTriggerTokens = *p.CompactionTriggerTokens
	}
	if p.CompactionKeepRecent != nil && *p.CompactionKeepRecent > 0 {
		c.CompactionKeepRecent = *p.CompactionKeepRecent
	}
	if p.CompactionMaxToolOutput != nil && *p.CompactionMaxToolOutput > 0 {
		c.CompactionMaxToolOutput = *p.CompactionMaxToolOutput
	}
	if p.BudgetDailyCap != nil && *p.BudgetDailyCap >= 0 {
		c.BudgetDailyCap = *p.BudgetDailyCap
	}
	if p.BudgetWeeklyCap != nil && *p.BudgetWeeklyCap >= 0 {
		c.BudgetWeeklyCap = *p.BudgetWeeklyCap
	}
	if p.ChannelSessionTimeout != nil && *p.ChannelSessionTimeout > 0 {
		c.ChannelSessionTimeout = *p.ChannelSessionTimeout
	}
	if p.GHOwner != nil {
		c.GHOwner = *p.GHOwner
	}
	if p.GHTrackedRepos != nil {
		if *p.GHTrackedRepos == "" {
			c.GHTrackedRepos = nil
		} else {
			c.GHTrackedRepos = splitTrimmed(*p.GHTrackedRepos)
		}
	}
	if p.GHBotFilter != nil {
		if *p.GHBotFilter == "" {
			c.GHBotFilter = nil
		} else {
			c.GHBotFilter = splitTrimmed(*p.GHBotFilter)
		}
	}
	if p.GHMetricsInterval != nil && *p.GHMetricsInterval > 0 {
		c.GHMetricsInterval = *p.GHMetricsInterval
	}
	if p.GmailEnabled != nil {
		c.GmailEnabled = *p.GmailEnabled
	}
	if p.CalendarEnabled != nil {
		c.CalendarEnabled = *p.CalendarEnabled
	}
	if p.GmailFilterQuery != nil {
		c.GmailFilterQuery = *p.GmailFilterQuery
	}
	if p.CalendarLookaheadH != nil && *p.CalendarLookaheadH > 0 {
		c.CalendarLookaheadH = *p.CalendarLookaheadH
	}
	if p.PreferredChannel != nil {
		c.PreferredChannel = *p.PreferredChannel
	}
	if p.QuietHoursStart != nil && (*p.QuietHoursStart == -1 || (*p.QuietHoursStart >= 0 && *p.QuietHoursStart <= 23)) {
		c.QuietHoursStart = *p.QuietHoursStart
	}
	if p.QuietHoursEnd != nil && (*p.QuietHoursEnd == -1 || (*p.QuietHoursEnd >= 0 && *p.QuietHoursEnd <= 23)) {
		c.QuietHoursEnd = *p.QuietHoursEnd
	}
	if p.QuietHoursTZ != nil && *p.QuietHoursTZ != "" {
		c.QuietHoursTZ = *p.QuietHoursTZ
	}
	if p.MutedSources != nil {
		if *p.MutedSources == "" {
			c.MutedSources = nil
		} else {
			c.MutedSources = splitTrimmed(*p.MutedSources)
		}
	}
	if p.NotifMinPriority != nil {
		prio := *p.NotifMinPriority
		if prio == "low" || prio == "medium" || prio == "high" {
			c.NotifMinPriority = prio
		}
	}
	if p.ChannelRouting != nil {
		c.ChannelRouting = *p.ChannelRouting
	}
	if p.RSSEnabled != nil {
		c.RSSEnabled = *p.RSSEnabled
	}
	if p.RSSFeeds != nil {
		if *p.RSSFeeds == "" {
			c.RSSFeeds = nil
		} else {
			c.RSSFeeds = splitTrimmed(*p.RSSFeeds)
		}
	}
	if p.SOEnabled != nil {
		c.SOEnabled = *p.SOEnabled
	}
	if p.SOTags != nil {
		if *p.SOTags == "" {
			c.SOTags = nil
		} else {
			c.SOTags = splitTrimmed(*p.SOTags)
		}
	}
	if p.DevToEnabled != nil {
		c.DevToEnabled = *p.DevToEnabled
	}
	if p.DevToTags != nil {
		if *p.DevToTags == "" {
			c.DevToTags = nil
		} else {
			c.DevToTags = splitTrimmed(*p.DevToTags)
		}
	}
	if p.DevToUsername != nil {
		c.DevToUsername = *p.DevToUsername
	}
	if p.NPMPackages != nil {
		if *p.NPMPackages == "" {
			c.NPMPackages = nil
		} else {
			c.NPMPackages = splitTrimmed(*p.NPMPackages)
		}
	}
	if p.CratesPackages != nil {
		if *p.CratesPackages == "" {
			c.CratesPackages = nil
		} else {
			c.CratesPackages = splitTrimmed(*p.CratesPackages)
		}
	}
	if p.MCPClientServers != nil {
		if *p.MCPClientServers == "" || *p.MCPClientServers == "[]" {
			c.MCPClientServers = nil
		} else {
			c.MCPClientServers = json.RawMessage(*p.MCPClientServers)
		}
	}
}

// GetPatchable returns the current runtime-editable config values.
func (c *Config) GetPatchable() PatchableConfig {
	configMu.RLock()
	defer configMu.RUnlock()
	return PatchableConfig{
		CompactionTriggerTokens: &c.CompactionTriggerTokens,
		CompactionKeepRecent:    &c.CompactionKeepRecent,
		CompactionMaxToolOutput: &c.CompactionMaxToolOutput,
		BudgetDailyCap:          &c.BudgetDailyCap,
		BudgetWeeklyCap:         &c.BudgetWeeklyCap,
		ChannelSessionTimeout:   &c.ChannelSessionTimeout,
		GHOwner:                 &c.GHOwner,
		GHTrackedRepos:          strPtr(strings.Join(c.GHTrackedRepos, ", ")),
		GHBotFilter:             strPtr(strings.Join(c.GHBotFilter, ", ")),
		GHMetricsInterval:       &c.GHMetricsInterval,
		GmailEnabled:            &c.GmailEnabled,
		CalendarEnabled:         &c.CalendarEnabled,
		GmailFilterQuery:        &c.GmailFilterQuery,
		CalendarLookaheadH:      &c.CalendarLookaheadH,
		PreferredChannel:        &c.PreferredChannel,
		QuietHoursStart:         &c.QuietHoursStart,
		QuietHoursEnd:           &c.QuietHoursEnd,
		QuietHoursTZ:            &c.QuietHoursTZ,
		MutedSources:            strPtr(strings.Join(c.MutedSources, ", ")),
		NotifMinPriority:        &c.NotifMinPriority,
		ChannelRouting:          &c.ChannelRouting,
		RSSEnabled:              &c.RSSEnabled,
		RSSFeeds:                strPtr(strings.Join(c.RSSFeeds, ", ")),
		SOEnabled:               &c.SOEnabled,
		SOTags:                  strPtr(strings.Join(c.SOTags, ", ")),
		DevToEnabled:            &c.DevToEnabled,
		DevToTags:               strPtr(strings.Join(c.DevToTags, ", ")),
		DevToUsername:           &c.DevToUsername,
		NPMPackages:             strPtr(strings.Join(c.NPMPackages, ", ")),
		CratesPackages:          strPtr(strings.Join(c.CratesPackages, ", ")),
		MCPClientServers: func() *string {
			if len(c.MCPClientServers) == 0 {
				return strPtr("[]")
			}
			s := string(c.MCPClientServers)
			return &s
		}(),
	}
}

func strPtr(s string) *string { return &s }

// MaxRoundsForMode returns the configured tool round limit for the given mode string.
func (c *Config) MaxRoundsForMode(mode string) int {
	switch mode {
	case "talk":
		return c.TalkMaxRounds
	case "work":
		return c.WorkMaxRounds
	case "coding":
		return c.CodingMaxRounds
	default:
		return c.TalkMaxRounds
	}
}

// splitTrimmed splits a comma-separated string and trims whitespace from each element.
func splitTrimmed(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// SaveOverrides writes the current patchable config to $dataDir/config.json.
func (c *Config) SaveOverrides(dataDir string) error {
	p := c.GetPatchable()
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "config.json"), data, 0644)
}

// LoadOverrides reads config.json from dataDir and applies over current config.
func (c *Config) LoadOverrides(dataDir string) {
	path := filepath.Join(dataDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return // no overrides file, that's fine
	}
	var p PatchableConfig
	if err := json.Unmarshal(data, &p); err != nil {
		fmt.Fprintf(os.Stderr, "warning: config.json invalid, ignoring: %v\n", err)
		return
	}
	c.ApplyPatch(p)
}
