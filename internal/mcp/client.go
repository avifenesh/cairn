package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/tool"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPServerConfig describes one external MCP server to connect to.
type MCPServerConfig struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport"` // "stdio" or "http"
	Command   string            `json:"command"`   // stdio only
	Args      []string          `json:"args"`      // stdio only
	Env       []string          `json:"env"`       // stdio only
	URL       string            `json:"url"`       // http only
	Headers   map[string]string `json:"headers"`   // http only
	Enabled   bool              `json:"enabled"`
}

// ConnectionStatus describes the current state of one MCP connection.
type ConnectionStatus struct {
	Name        string     `json:"name"`
	Transport   string     `json:"transport"`
	Status      string     `json:"status"` // "connected", "connecting", "disconnected", "error"
	ToolCount   int        `json:"toolCount"`
	Error       string     `json:"error,omitempty"`
	ConnectedAt *time.Time `json:"connectedAt,omitempty"`
}

// connection tracks a single MCP client connection and its tools.
type connection struct {
	config      MCPServerConfig
	client      *client.Client
	tools       []string // registered tool names (for cleanup)
	status      string
	err         error
	connectedAt time.Time
	mu          sync.Mutex
}

// ClientManager manages connections to external MCP servers.
type ClientManager struct {
	connections map[string]*connection
	registry    *tool.Registry
	bus         *eventbus.Bus
	logger      *slog.Logger
	mu          sync.RWMutex
}

// NewClientManager creates a new MCP client manager.
func NewClientManager(registry *tool.Registry, bus *eventbus.Bus, logger *slog.Logger) *ClientManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &ClientManager{
		connections: make(map[string]*connection),
		registry:    registry,
		bus:         bus,
		logger:      logger,
	}
}

// Connect establishes a connection to an external MCP server.
func (m *ClientManager) Connect(ctx context.Context, cfg MCPServerConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("mcp client: server name is required")
	}
	if cfg.Transport != "stdio" && cfg.Transport != "http" {
		return fmt.Errorf("mcp client: transport must be 'stdio' or 'http', got %q", cfg.Transport)
	}

	m.mu.Lock()
	if _, exists := m.connections[cfg.Name]; exists {
		m.mu.Unlock()
		return fmt.Errorf("mcp client: server %q already connected", cfg.Name)
	}
	conn := &connection{
		config: cfg,
		status: "connecting",
	}
	m.connections[cfg.Name] = conn
	m.mu.Unlock()

	m.publishStatus(cfg.Name, "connecting", 0, "")

	if err := m.connectOne(ctx, conn); err != nil {
		conn.mu.Lock()
		conn.status = "error"
		conn.err = err
		conn.mu.Unlock()
		m.publishStatus(cfg.Name, "error", 0, err.Error())
		return fmt.Errorf("mcp client %s: %w", cfg.Name, err)
	}

	return nil
}

// connectOne does the actual connection, initialization, and tool discovery.
func (m *ClientManager) connectOne(ctx context.Context, conn *connection) error {
	cfg := conn.config
	var c *client.Client
	var err error

	switch cfg.Transport {
	case "stdio":
		if cfg.Command == "" {
			return fmt.Errorf("command is required for stdio transport")
		}
		c, err = client.NewStdioMCPClient(cfg.Command, cfg.Env, cfg.Args...)
		if err != nil {
			return fmt.Errorf("create stdio client: %w", err)
		}
	case "http":
		if cfg.URL == "" {
			return fmt.Errorf("url is required for http transport")
		}
		var opts []transport.StreamableHTTPCOption
		if len(cfg.Headers) > 0 {
			opts = append(opts, transport.WithHTTPHeaders(cfg.Headers))
		}
		c, err = client.NewStreamableHttpClient(cfg.URL, opts...)
		if err != nil {
			return fmt.Errorf("create http client: %w", err)
		}
		if err := c.Start(ctx); err != nil {
			c.Close()
			return fmt.Errorf("start http client: %w", err)
		}
	}

	// Initialize handshake.
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "cairn",
		Version: "1.0.0",
	}

	initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
	defer initCancel()

	_, err = c.Initialize(initCtx, initReq)
	if err != nil {
		c.Close()
		return fmt.Errorf("initialize: %w", err)
	}

	// Discover tools.
	toolsResult, err := c.ListTools(initCtx, mcp.ListToolsRequest{})
	if err != nil {
		c.Close()
		return fmt.Errorf("list tools: %w", err)
	}

	// Wrap and register tools.
	wrappedTools := wrapMCPTools(cfg.Name, toolsResult.Tools, c)
	toolNames := make([]string, len(wrappedTools))
	for i, t := range wrappedTools {
		toolNames[i] = t.Name()
	}
	m.registry.Register(wrappedTools...)

	conn.mu.Lock()
	conn.client = c
	conn.tools = toolNames
	conn.status = "connected"
	conn.err = nil
	conn.connectedAt = time.Now()
	conn.mu.Unlock()

	m.publishStatus(cfg.Name, "connected", len(toolNames), "")
	m.logger.Info("mcp client connected",
		"server", cfg.Name,
		"transport", cfg.Transport,
		"tools", len(toolNames),
	)

	return nil
}

// Disconnect closes a connection and deregisters its tools.
func (m *ClientManager) Disconnect(name string) error {
	m.mu.Lock()
	conn, ok := m.connections[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("mcp client: server %q not found", name)
	}
	delete(m.connections, name)
	m.mu.Unlock()

	m.cleanupConnection(conn)
	m.publishStatus(name, "disconnected", 0, "")
	m.logger.Info("mcp client disconnected", "server", name)
	return nil
}

// Reconnect drops and re-establishes a connection.
func (m *ClientManager) Reconnect(name string) error {
	m.mu.RLock()
	conn, ok := m.connections[name]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("mcp client: server %q not found", name)
	}
	cfg := conn.config
	m.mu.RUnlock()

	// Clean up old connection.
	m.cleanupConnection(conn)
	m.publishStatus(name, "connecting", 0, "")

	// Reset the connection for reconnect.
	conn.mu.Lock()
	conn.client = nil
	conn.tools = nil
	conn.status = "connecting"
	conn.err = nil
	conn.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := m.connectOne(ctx, conn); err != nil {
		conn.mu.Lock()
		conn.status = "error"
		conn.err = err
		conn.mu.Unlock()
		m.publishStatus(cfg.Name, "error", 0, err.Error())
		return err
	}
	return nil
}

// ConnectAll connects to all configured servers. Errors are logged, not returned.
func (m *ClientManager) ConnectAll(ctx context.Context, configs []MCPServerConfig) {
	for _, cfg := range configs {
		if !cfg.Enabled {
			m.logger.Debug("mcp client: skipping disabled server", "name", cfg.Name)
			continue
		}
		if err := m.Connect(ctx, cfg); err != nil {
			m.logger.Warn("mcp client: failed to connect",
				"server", cfg.Name,
				"error", err,
			)
		}
	}
}

// Status returns the current status of all connections.
func (m *ClientManager) Status() []ConnectionStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]ConnectionStatus, 0, len(m.connections))
	for _, conn := range m.connections {
		statuses = append(statuses, m.connStatus(conn))
	}
	return statuses
}

// ConnectionStatus returns the status of a single connection.
func (m *ClientManager) ConnectionStatus(name string) (*ConnectionStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.connections[name]
	if !ok {
		return nil, false
	}
	s := m.connStatus(conn)
	return &s, true
}

func (m *ClientManager) connStatus(conn *connection) ConnectionStatus {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	s := ConnectionStatus{
		Name:      conn.config.Name,
		Transport: conn.config.Transport,
		Status:    conn.status,
		ToolCount: len(conn.tools),
	}
	if conn.err != nil {
		s.Error = conn.err.Error()
	}
	if !conn.connectedAt.IsZero() {
		t := conn.connectedAt
		s.ConnectedAt = &t
	}
	return s
}

// Close shuts down all connections gracefully.
func (m *ClientManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, conn := range m.connections {
		m.cleanupConnection(conn)
		m.logger.Debug("mcp client closed", "server", name)
	}
	m.connections = make(map[string]*connection)
}

// cleanupConnection deregisters tools and closes the client.
func (m *ClientManager) cleanupConnection(conn *connection) {
	conn.mu.Lock()
	tools := conn.tools
	c := conn.client
	conn.tools = nil
	conn.client = nil
	conn.mu.Unlock()

	for _, name := range tools {
		m.registry.Deregister(name)
	}
	if c != nil {
		c.Close()
	}
}

func (m *ClientManager) publishStatus(name, status string, toolCount int, errMsg string) {
	if m.bus == nil {
		return
	}
	eventbus.Publish(m.bus, eventbus.MCPConnectionChanged{
		EventMeta:  eventbus.NewMeta("mcp-client"),
		ServerName: name,
		Status:     status,
		ToolCount:  toolCount,
		Error:      errMsg,
	})
}

// ParseServerConfigs parses a JSON array of MCP server configs.
func ParseServerConfigs(raw json.RawMessage) ([]MCPServerConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var configs []MCPServerConfig
	if err := json.Unmarshal(raw, &configs); err != nil {
		return nil, fmt.Errorf("parse MCP_SERVERS: %w", err)
	}
	return configs, nil
}
