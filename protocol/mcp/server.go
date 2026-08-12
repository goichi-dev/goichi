package mcp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"github.com/goichi-dev/goichi/middleware"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type Tool struct {
	Name        string                                             `json:"name"`
	Description string                                             `json:"description"`
	InputSchema map[string]any                                     `json:"inputSchema"`
	Handler     func(context.Context, map[string]any) (any, error) `json:"-"`
}

type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
	Content     any    `json:"content,omitempty"`
}

type Prompt struct {
	Name        string                                                `json:"name"`
	Description string                                                `json:"description"`
	Arguments   []PromptArgument                                      `json:"arguments"`
	Handler     func(context.Context, map[string]any) (string, error) `json:"-"`
}

type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type MCPSession struct {
	ID            string
	conn          net.Conn
	ctx           context.Context
	cancel        context.CancelFunc
	metadata      map[string]any
	authenticated bool
}

// errRequestTooLarge is returned when a single JSON-RPC request exceeds the
// configured size cap.
var errRequestTooLarge = errors.New("request exceeds maximum size")

// limitedReader caps the number of bytes that may be consumed for a single
// message. Reset between messages via reset().
type limitedReader struct {
	r   io.Reader
	n   int64
	max int64
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.n >= l.max {
		return 0, errRequestTooLarge
	}
	if int64(len(p)) > l.max-l.n {
		p = p[:l.max-l.n]
	}
	n, err := l.r.Read(p)
	l.n += int64(n)
	return n, err
}

func (l *limitedReader) reset() { l.n = 0 }

type MCPServer struct {
	config    MCPConfig
	listener  net.Listener
	isRunning bool
	tools     map[string]*Tool
	resources map[string]*Resource
	prompts   map[string]*Prompt
	sessions  map[string]*MCPSession
	mu        sync.RWMutex
	done      chan struct{}

	authSecret string
	authLookup string
}

func (ms *MCPServer) SetAuthSecret(secret string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.authSecret = secret
}

func (ms *MCPServer) SetAuthLookup(lookup string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.authLookup = lookup
}

func (ms *MCPServer) SetListener(ln net.Listener) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.listener = ln
}

func NewMCPServer(config MCPConfig) *MCPServer {
	config.SetDefaults()
	return &MCPServer{
		config:    config,
		tools:     make(map[string]*Tool),
		resources: make(map[string]*Resource),
		prompts:   make(map[string]*Prompt),
		sessions:  make(map[string]*MCPSession),
		done:      make(chan struct{}),
	}
}

func (ms *MCPServer) GetInfo() string {
	return fmt.Sprintf("MCP AI Agent Protocol (Port %d)", ms.config.Port)
}

func (ms *MCPServer) GetName() string {
	return "mcp"
}

func (ms *MCPServer) GetPaths() []string {
	return nil
}

func (ms *MCPServer) RegisterTool(tool *Tool) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if len(ms.tools) >= ms.config.MaxToolCount {
		return fmt.Errorf("maximum tool count (%d) reached", ms.config.MaxToolCount)
	}

	ms.tools[tool.Name] = tool
	return nil
}

func (ms *MCPServer) RegisterResource(resource *Resource) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if len(ms.resources) >= ms.config.MaxResourceCount {
		return fmt.Errorf("maximum resource count (%d) reached", ms.config.MaxResourceCount)
	}

	ms.resources[resource.URI] = resource
	return nil
}

func (ms *MCPServer) RegisterPrompt(prompt *Prompt) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.prompts[prompt.Name] = prompt
}

func (ms *MCPServer) Start(ctx context.Context) error {
	ms.mu.Lock()
	if ms.isRunning {
		ms.mu.Unlock()
		return nil
	}
	ms.mu.Unlock()

	if !ms.config.Enabled {
		log.Printf("[MCP] Disabled, skipping start")
		return nil
	}

	addr := fmt.Sprintf("%s:%d", ms.config.Address, ms.config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	if ms.config.TLSEnabled() {
		cert, err := tls.LoadX509KeyPair(ms.config.TLSCertFile, ms.config.TLSKeyFile)
		if err != nil {
			listener.Close()
			return fmt.Errorf("mcp tls: %w", err)
		}
		listener = tls.NewListener(listener, &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		})
	}

	ms.listener = listener
	ms.isRunning = true

	go ms.acceptConnections()

	log.Printf("[MCP] Server started on %s", addr)
	return nil
}

func (ms *MCPServer) Stop(ctx context.Context) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if !ms.isRunning {
		return nil
	}

	for _, session := range ms.sessions {
		session.conn.Close()
		session.cancel()
	}

	if ms.listener != nil {
		ms.listener.Close()
	}

	ms.isRunning = false
	close(ms.done)

	log.Printf("[MCP] Server stopped")
	return nil
}

func (ms *MCPServer) IsRunning() bool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.isRunning
}

func (ms *MCPServer) acceptConnections() {
	for {
		conn, err := ms.listener.Accept()
		if err != nil {
			select {
			case <-ms.done:
				return
			default:
				log.Printf("[MCP] Accept error: %v", err)
			}
			continue
		}

		go ms.handleConnection(conn)
	}
}

func (ms *MCPServer) handleConnection(conn net.Conn) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionID := fmt.Sprintf("session-%s", conn.RemoteAddr())

	session := &MCPSession{
		ID:       sessionID,
		conn:     conn,
		ctx:      ctx,
		cancel:   cancel,
		metadata: make(map[string]any),
	}

	ms.mu.Lock()
	ms.sessions[sessionID] = session
	ms.mu.Unlock()

	defer func() {
		conn.Close()
		ms.mu.Lock()
		delete(ms.sessions, sessionID)
		ms.mu.Unlock()
		cancel()
	}()

	lr := &limitedReader{r: conn, max: ms.config.MaxRequestBytes}
	decoder := json.NewDecoder(lr)
	encoder := json.NewEncoder(conn)

	idle := time.Duration(ms.config.Timeout) * time.Second

	for {
		if idle > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(idle))
		}
		lr.reset()

		var req JSONRPCRequest
		if err := decoder.Decode(&req); err != nil {
			if err != io.EOF {
				log.Printf("[MCP] Decode error: %v", err)
			}
			return
		}

		resp := ms.handleRequest(ctx, session, req)
		if err := encoder.Encode(resp); err != nil {
			log.Printf("[MCP] Encode error: %v", err)
			return
		}
	}
}

func (ms *MCPServer) handleRequest(ctx context.Context, session *MCPSession, req JSONRPCRequest) JSONRPCResponse {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	ms.mu.RLock()
	secret := ms.authSecret
	required := ms.config.RequireAuth
	ms.mu.RUnlock()
	authNeeded := secret != "" || required

	// "initialize" is the only method allowed before authentication; it carries
	// the credential (params.token) when unified auth is enabled.
	if req.Method == "initialize" {
		if authNeeded {
			var p struct {
				Token string `json:"token"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if secret == "" {
				resp.Error = map[string]any{"code": -32001, "message": "auth not configured"}
				return resp
			}
			if _, errMsg := middleware.ValidateJWT(p.Token, secret); errMsg != "" {
				resp.Error = map[string]any{"code": -32001, "message": "unauthorized"}
				return resp
			}
			session.authenticated = true
		}
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": true},
				"resources": map[string]any{"subscribe": true, "listChanged": true},
				"prompts":   map[string]any{"listChanged": true},
			},
			"serverInfo": map[string]string{
				"name":    "Goichi MCP Server",
				"version": "1.0.0",
			},
		}
		return resp
	}

	// Every other method requires a successful initialize+auth first.
	if authNeeded && !session.authenticated {
		resp.Error = map[string]any{"code": -32001, "message": "unauthorized"}
		return resp
	}

	switch req.Method {
	case "tools/list":
		ms.mu.RLock()
		tools := make([]*Tool, 0, len(ms.tools))
		for _, t := range ms.tools {
			tools = append(tools, t)
		}
		ms.mu.RUnlock()
		resp.Result = map[string]any{"tools": tools}
	case "resources/list":
		ms.mu.RLock()
		resources := make([]*Resource, 0, len(ms.resources))
		for _, r := range ms.resources {
			resources = append(resources, r)
		}
		ms.mu.RUnlock()
		resp.Result = map[string]any{"resources": resources}
	case "prompts/list":
		ms.mu.RLock()
		prompts := make([]*Prompt, 0, len(ms.prompts))
		for _, p := range ms.prompts {
			prompts = append(prompts, p)
		}
		ms.mu.RUnlock()
		resp.Result = map[string]any{"prompts": prompts}
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = map[string]any{"code": -32602, "message": "Invalid params"}
			return resp
		}

		ms.mu.RLock()
		tool, ok := ms.tools[params.Name]
		ms.mu.RUnlock()
		if !ok {
			resp.Error = map[string]any{"code": -32601, "message": "Tool not found"}
			return resp
		}

		// Call the handler WITHOUT holding the registry lock so a slow or
		// re-entrant tool cannot stall connection handling or deadlock.
		result, err := tool.Handler(ctx, params.Arguments)
		if err != nil {
			resp.Error = map[string]any{"code": -32000, "message": err.Error()}
			return resp
		}
		resp.Result = result
	default:
		resp.Error = map[string]any{"code": -32601, "message": "Method not found"}
	}

	return resp
}

func (ms *MCPServer) GetSessions() map[string]*MCPSession {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	sessions := make(map[string]*MCPSession)
	for id, session := range ms.sessions {
		sessions[id] = session
	}
	return sessions
}

func (ms *MCPServer) GetTools() map[string]*Tool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.tools
}

func (ms *MCPServer) GetResources() map[string]*Resource {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.resources
}

func (ms *MCPServer) GetPrompts() map[string]*Prompt {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.prompts
}

func (ms *MCPServer) ExecuteTool(ctx context.Context, name string, args map[string]any) (any, error) {
	ms.mu.RLock()
	tool, ok := ms.tools[name]
	ms.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool not found")
	}

	return tool.Handler(ctx, args)
}
