package llm

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// Registry manages multiple LLM providers and resolves models to providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider              // providerID → Provider
	models    map[string]string                // modelID → providerID
	fallbacks map[string]string                // modelID → fallback modelID
	defaults  struct{ provider, model string } // default provider + model
	logger    *slog.Logger
}

// NewRegistry creates an empty provider registry.
func NewRegistry(logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	return &Registry{
		providers: make(map[string]Provider),
		models:    make(map[string]string),
		fallbacks: make(map[string]string),
		logger:    logger,
	}
}

// Register adds a provider and registers all its models.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[p.ID()] = p
	for _, m := range p.Models() {
		r.models[m.ID] = p.ID()
	}

	// First registered provider becomes the default.
	if r.defaults.provider == "" {
		r.defaults.provider = p.ID()
		if models := p.Models(); len(models) > 0 {
			r.defaults.model = models[0].ID
		}
	}

	r.logger.Info("llm: provider registered",
		"provider", p.ID(),
		"models", len(p.Models()),
	)
}

// SetDefault sets the default provider and model.
func (r *Registry) SetDefault(providerID, modelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaults.provider = providerID
	r.defaults.model = modelID
}

// SetFallback configures a fallback model for a given model.
// When the primary model fails, the retry wrapper can try the fallback.
func (r *Registry) SetFallback(modelID, fallbackModelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallbacks[modelID] = fallbackModelID
}

// Resolve finds the provider for a model ID.
// If modelID is empty, returns the default.
// Returns the provider and the resolved model ID.
func (r *Registry) Resolve(modelID string) (Provider, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if modelID == "" {
		modelID = r.defaults.model
	}

	providerID, ok := r.models[modelID]
	if !ok {
		return nil, "", fmt.Errorf("llm: unknown model %q", modelID)
	}

	provider, ok := r.providers[providerID]
	if !ok {
		return nil, "", fmt.Errorf("llm: provider %q not found for model %q", providerID, modelID)
	}

	return provider, modelID, nil
}

// Default returns the default provider.
func (r *Registry) Default() (Provider, string, error) {
	return r.Resolve("")
}

// Provider returns a provider by ID.
func (r *Registry) Provider(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// FallbackFor returns the fallback model for a given model, if configured.
func (r *Registry) FallbackFor(modelID string) (Provider, string, bool) {
	r.mu.RLock()
	fallbackModel, ok := r.fallbacks[modelID]
	r.mu.RUnlock()

	if !ok {
		return nil, "", false
	}

	provider, resolved, err := r.Resolve(fallbackModel)
	if err != nil {
		return nil, "", false
	}
	return provider, resolved, true
}

// WithRetryAndFallback wraps the resolved provider for a model with retry logic
// and automatic fallback if configured. Supports fallback chains: if model A
// falls back to B, and B falls back to C, each level gets its own retry wrapper.
func (r *Registry) WithRetryAndFallback(modelID string, config RetryConfig) (Provider, string, error) {
	primary, resolved, err := r.Resolve(modelID)
	if err != nil {
		return nil, "", err
	}

	// Build the fallback chain from the bottom up. Walk the chain first to
	// collect all levels, then wrap from the tail so each fallback is itself
	// a retry+fallback provider.
	type level struct {
		provider Provider
		modelID  string
	}

	chain := []level{{primary, resolved}}
	seen := map[string]bool{resolved: true}
	cur := resolved
	for {
		fb, fbModel, ok := r.FallbackFor(cur)
		if !ok {
			break
		}
		if seen[fbModel] {
			break // prevent cycles
		}
		seen[fbModel] = true
		chain = append(chain, level{fb, fbModel})
		cur = fbModel
	}

	// Wrap from tail to head. The last level has no fallback.
	var wrapped Provider
	for i := len(chain) - 1; i >= 0; i-- {
		opts := []RetryOption{WithLogger(r.logger)}
		if wrapped != nil {
			opts = append(opts, WithFallback(wrapped))
		}
		wrapped = WithRetry(chain[i].provider, config, opts...)
	}

	if len(chain) > 1 {
		names := make([]string, len(chain))
		for i, l := range chain {
			names[i] = l.modelID
		}
		r.logger.Info("llm: fallback chain configured", "chain", names)
	}

	return wrapped, resolved, nil
}

// ProviderConfig holds configuration for initializing a provider.
type ProviderConfig struct {
	Type    string // "glm", "openai"
	APIKey  string
	BaseURL string
	Model   string
}

// RegisterFromConfig creates and registers a provider from config.
func (r *Registry) RegisterFromConfig(cfg ProviderConfig) error {
	var p Provider

	switch strings.ToLower(cfg.Type) {
	case "glm", "zhipu":
		p = NewGLMProvider(cfg.APIKey, cfg.BaseURL, cfg.Model)
	case "openai", "openai-compatible":
		p = NewOpenAIProvider(cfg.APIKey, cfg.BaseURL, cfg.Model)
	default:
		return fmt.Errorf("llm: unknown provider type %q", cfg.Type)
	}

	r.Register(p)
	return nil
}

// ListProviders returns all registered provider IDs.
func (r *Registry) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	return ids
}

// ListModels returns all registered model IDs.
func (r *Registry) ListModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.models))
	for id := range r.models {
		ids = append(ids, id)
	}
	return ids
}
