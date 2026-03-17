package config

import (
	"encoding/json"
	"fmt"
	"os"
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
	DigestEnabled  bool              // Enable periodic digest generation

	// Paths
	SoulPath  string
	SkillDirs []string
	DataDir   string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	// Read provider-agnostic vars first, fall back to GLM-specific aliases.
	apiKey := envStr("LLM_API_KEY", envStr("GLM_API_KEY", envStr("OPENAI_API_KEY", "")))
	baseURL := envStr("LLM_BASE_URL", envStr("GLM_BASE_URL", envStr("OPENAI_BASE_URL", "")))
	model := envStr("LLM_MODEL", envStr("GLM_MODEL", ""))
	provider := envStr("LLM_PROVIDER", "")

	// Auto-detect provider from env vars if not explicitly set.
	if provider == "" {
		switch {
		case envStr("GLM_API_KEY", "") != "":
			provider = "glm"
		case envStr("OPENAI_API_KEY", "") != "":
			provider = "openai"
		default:
			provider = "glm" // default
		}
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
		Port:             envInt("PORT", 8787),
		Host:             envStr("HOST", "0.0.0.0"),
		DatabasePath:     envStr("DATABASE_PATH", "./data/cairn.db"),
		LLMProvider:      provider,
		LLMAPIKey:        apiKey,
		LLMBaseURL:       baseURL,
		LLMModel:         model,
		LLMFallbackModel: envStr("LLM_FALLBACK_MODEL", envStr("GLM_FALLBACK_MODEL", "")),
		GLMAPIKey:        apiKey,
		GLMBaseURL:       baseURL,
		GLMModel:         model,
		WriteAPIToken:    envStr("WRITE_API_TOKEN", ""),
		ReadAPIToken:     envStr("READ_API_TOKEN", ""),
		FrontendOrigin:   envStr("FRONTEND_ORIGIN", ""),
		CodingEnabled:    envBool("CODING_ENABLED", false),
		IdleModeEnabled:  envBool("IDLE_MODE_ENABLED", false),
		GHToken:          envStr("GH_TOKEN", envStr("GITHUB_TOKEN", "")),
		GHOrgs:           envSlice("GH_ORGS", nil),
		HNKeywords:       envSlice("HN_KEYWORDS", nil),
		HNMinScore:       envInt("HN_MIN_SCORE", 0),
		PollInterval:     envInt("POLL_INTERVAL", 300),
		RedditSubs:       envSlice("REDDIT_SUBS", nil),
		NPMPackages:      envSlice("NPM_PACKAGES", nil),
		CratesPackages:   envSlice("CRATES_PACKAGES", nil),
		WebhookSecrets:   envMap("WEBHOOK_SECRETS"),
		DigestEnabled:    envBool("DIGEST_ENABLED", false),
		SoulPath:         envStr("SOUL_PATH", "./SOUL.md"),
		SkillDirs:        envSlice("SKILL_DIRS", []string{"./.pub/skills"}),
		DataDir:          envStr("DATA_DIR", "./data"),
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
			SkillDirs:    envSlice("SKILL_DIRS", []string{"./.pub/skills"}),
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

// envMap parses JSON from an env var into map[string]string.
// Example: WEBHOOK_SECRETS='{"github":"abc123","stripe":"xyz789"}'
func envMap(key string) map[string]string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(v), &m); err != nil {
		return nil
	}
	return m
}
