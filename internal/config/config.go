package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	// Signal plane
	GHToken        string            // GitHub personal access token
	GHOrgs         []string          // GitHub orgs to track
	HNKeywords     []string          // HN keyword filter
	HNMinScore     int               // HN minimum score filter
	PollInterval   int               // Poll interval in seconds (default 300 = 5min)
	RedditSubs     []string          // Subreddits to monitor
	NPMPackages    []string          // npm packages to track
	CratesPackages []string          // crates.io crates to track
	WebhookSecrets map[string]string // webhook name -> HMAC secret

	// Memory context builder
	MemoryContextBudget   int     // Token budget (default: 4000)
	MemoryHardRuleReserve int     // Reserved for hard rules (default: 500)
	MemoryDecayHalfLife   float64 // Days (default: 30)
	MemoryStaleThreshold  float64 // Days (default: 14)

	// Budget
	BudgetDailyCap  float64 // Daily LLM spend cap USD (0 = unlimited)
	BudgetWeeklyCap float64 // Weekly LLM spend cap USD (0 = unlimited)

	// Agent loop
	AgentTickInterval  int // Seconds (default: 60)
	ReflectionInterval int // Seconds (default: 1800)

	// Web tools
	SearXNGURL      string // SearXNG instance URL for web search
	WebFetchTimeout int    // Seconds (default: 30)
	WebFetchMaxSize int64  // Bytes (default: 5MB)

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
		Port:                  envInt("PORT", 8787),
		Host:                  envStr("HOST", "0.0.0.0"),
		DatabasePath:          envStr("DATABASE_PATH", "./data/cairn.db"),
		LLMProvider:           provider,
		LLMAPIKey:             apiKey,
		LLMBaseURL:            baseURL,
		LLMModel:              model,
		LLMFallbackModel:      envStr("LLM_FALLBACK_MODEL", envStr("GLM_FALLBACK_MODEL", "")),
		GLMAPIKey:             apiKey,
		GLMBaseURL:            baseURL,
		GLMModel:              model,
		WriteAPIToken:         envStr("WRITE_API_TOKEN", ""),
		ReadAPIToken:          envStr("READ_API_TOKEN", ""),
		FrontendOrigin:        envStr("FRONTEND_ORIGIN", ""),
		CodingEnabled:         envBool("CODING_ENABLED", false),
		IdleModeEnabled:       envBool("IDLE_MODE_ENABLED", false),
		GHToken:               envStr("GH_TOKEN", envStr("GITHUB_TOKEN", "")),
		GHOrgs:                envSlice("GH_ORGS", nil),
		HNKeywords:            envSlice("HN_KEYWORDS", nil),
		HNMinScore:            envInt("HN_MIN_SCORE", 0),
		PollInterval:          pollIntervalSeconds(),
		RedditSubs:            envSlice("REDDIT_SUBS", nil),
		NPMPackages:           envSlice("NPM_PACKAGES", nil),
		CratesPackages:        envSlice("CRATES_PACKAGES", envSlice("CRATES", nil)),
		WebhookSecrets:        envMap("WEBHOOK_SECRETS"),
		MemoryContextBudget:   envInt("MEMORY_CONTEXT_BUDGET", 4000),
		MemoryHardRuleReserve: envInt("MEMORY_HARD_RULE_RESERVE", 500),
		MemoryDecayHalfLife:   envFloat("MEMORY_DECAY_HALF_LIFE", 30),
		MemoryStaleThreshold:  envFloat("MEMORY_STALE_THRESHOLD", 14),
		BudgetDailyCap:        envFloat("BUDGET_DAILY_CAP", envFloat("BEDROCK_DAILY_BUDGET_USD", envFloat("IDLE_BUDGET_CAP_USD", 0))),
		BudgetWeeklyCap:       envFloat("BUDGET_WEEKLY_CAP", envFloat("BEDROCK_WEEKLY_BUDGET_USD", 0)),
		AgentTickInterval:     envInt("AGENT_TICK_INTERVAL", 60),
		ReflectionInterval:    envInt("REFLECTION_INTERVAL", 1800),
		SearXNGURL:            envStr("SEARXNG_URL", ""),
		WebFetchTimeout:       envInt("WEB_FETCH_TIMEOUT", 30),
		WebFetchMaxSize:       envInt64("WEB_FETCH_MAX_SIZE", 5*1024*1024),
		SoulPath:              envStr("SOUL_PATH", "./SOUL.md"),
		SkillDirs:             skillDirs(),
		DataDir:               envStr("DATA_DIR", "./data"),
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
			Port:         envInt("PORT", 8787),
			Host:         envStr("HOST", "0.0.0.0"),
			DatabasePath: envStr("DATABASE_PATH", "./data/cairn.db"),
			LLMProvider:  envStr("LLM_PROVIDER", "glm"),
			SoulPath:     envStr("SOUL_PATH", "./SOUL.md"),
			SkillDirs:    skillDirs(),
			DataDir:      envStr("DATA_DIR", "./data"),
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
