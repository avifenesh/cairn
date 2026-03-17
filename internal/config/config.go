package config

import (
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

	// LLM
	GLMAPIKey  string
	GLMBaseURL string
	GLMModel   string

	// GLM Fallback
	GLMFallbackModel string

	// Auth tokens
	WriteAPIToken string
	ReadAPIToken  string

	// Frontend
	FrontendOrigin string

	// Feature flags
	CodingEnabled bool
	IdleModeEnabled bool

	// Paths
	SoulPath   string
	SkillDirs  []string
	DataDir    string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	c := &Config{
		Port:             envInt("PORT", 8787),
		Host:             envStr("HOST", "0.0.0.0"),
		DatabasePath:     envStr("DATABASE_PATH", "./data/pub.db"),
		GLMAPIKey:        envStr("GLM_API_KEY", ""),
		GLMBaseURL:       envStr("GLM_BASE_URL", "https://api.z.ai/api/coding/paas/v4"),
		GLMModel:         envStr("GLM_MODEL", "glm-5-turbo"),
		GLMFallbackModel: envStr("GLM_FALLBACK_MODEL", "glm-4.7"),
		WriteAPIToken:    envStr("WRITE_API_TOKEN", ""),
		ReadAPIToken:     envStr("READ_API_TOKEN", ""),
		FrontendOrigin:   envStr("FRONTEND_ORIGIN", ""),
		CodingEnabled:    envBool("CODING_ENABLED", false),
		IdleModeEnabled:  envBool("IDLE_MODE_ENABLED", false),
		SoulPath:         envStr("SOUL_PATH", "./SOUL.md"),
		SkillDirs:        envSlice("SKILL_DIRS", []string{"./.pub/skills"}),
		DataDir:          envStr("DATA_DIR", "./data"),
	}

	if c.GLMAPIKey == "" {
		return nil, fmt.Errorf("GLM_API_KEY is required")
	}

	return c, nil
}

// LoadOptional is like Load but does not error on missing API keys.
// Useful for testing or when LLM is not needed.
func LoadOptional() *Config {
	c, _ := Load()
	if c == nil {
		c = &Config{
			Port:             envInt("PORT", 8787),
			Host:             envStr("HOST", "0.0.0.0"),
			DatabasePath:     envStr("DATABASE_PATH", "./data/pub.db"),
			GLMAPIKey:        envStr("GLM_API_KEY", ""),
			GLMBaseURL:       envStr("GLM_BASE_URL", "https://api.z.ai/api/coding/paas/v4"),
			GLMModel:         envStr("GLM_MODEL", "glm-5-turbo"),
			GLMFallbackModel: envStr("GLM_FALLBACK_MODEL", "glm-4.7"),
			WriteAPIToken:    envStr("WRITE_API_TOKEN", ""),
			ReadAPIToken:     envStr("READ_API_TOKEN", ""),
			FrontendOrigin:   envStr("FRONTEND_ORIGIN", ""),
			SoulPath:         envStr("SOUL_PATH", "./SOUL.md"),
			SkillDirs:        envSlice("SKILL_DIRS", []string{"./.pub/skills"}),
			DataDir:          envStr("DATA_DIR", "./data"),
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
