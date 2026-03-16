package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/raeseoklee/a2a-sentinel/internal/i18n"
)

// Config holds MCP server configuration.
type Config struct {
	Enabled bool
	Host    string
	Port    int
	Token   string // empty means no auth required
}

// Server is an MCP management server bound to localhost.
type Server struct {
	bridge     SentinelBridge
	addr       string
	token      string
	mu         sync.Mutex
	httpServer *http.Server
	listener   net.Listener // if non-nil, Start uses this instead of creating one
	logger     *slog.Logger
	sessions   sync.Map // sessionID (string) -> struct{}
	bundle     *i18n.Bundle
}

// jsonRPCRequest is a minimal JSON-RPC 2.0 request envelope.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse is a minimal JSON-RPC 2.0 response envelope.
type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

// rpcError represents a JSON-RPC error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewServer creates an MCP server from configuration.
// bundle provides i18n translations for tool descriptions and error messages.
func NewServer(cfg Config, bridge SentinelBridge, logger *slog.Logger, bundle *i18n.Bundle) *Server {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	return &Server{
		bridge: bridge,
		addr:   addr,
		token:  cfg.Token,
		logger: logger,
		bundle: bundle,
	}
}

// Start begins listening on the configured address. It blocks until ctx is
// cancelled or a fatal error occurs.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRPC)

	// Use injected listener or create one
	ln := s.listener
	if ln == nil {
		var err error
		ln, err = net.Listen("tcp", s.addr)
		if err != nil {
			return fmt.Errorf("mcp server listen %s: %w", s.addr, err)
		}
	}

	srv := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	s.mu.Lock()
	s.httpServer = srv
	s.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("mcp server listening", "addr", s.addr)
		errCh <- srv.Serve(ln)
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("mcp server error: %w", err)
		}
	case <-ctx.Done():
		s.logger.Info("mcp server shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	return s.Shutdown(shutdownCtx)
}

// Shutdown gracefully stops the MCP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpServer
	s.mu.Unlock()

	if srv != nil {
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("mcp server shutdown: %w", err)
		}
	}
	return nil
}

// generateSessionID returns a crypto-random hex-encoded 16-byte session ID.
func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// isNotification returns true if the decoded request has no id field (a JSON-RPC
// notification). Per MCP spec, id MUST NOT be null, so nil id is treated as absent.
func isNotification(req jsonRPCRequest) bool {
	return req.ID == nil
}

// handleRPC is the single HTTP endpoint that processes all JSON-RPC requests.
func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	// Create localizer from Accept-Language header for i18n support.
	localizer := s.bundle.NewLocalizer(r.Header.Get("Accept-Language"))

	// Only POST is valid for JSON-RPC over Streamable HTTP.
	if r.Method != http.MethodPost {
		http.Error(w, localizer.T(i18n.ErrMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	// Three-state auth: no header → anonymous, valid → authenticated, invalid → reject.
	authenticated := false
	if s.token != "" {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			expected := "Bearer " + s.token
			if strings.EqualFold(strings.TrimSpace(authHeader), expected) {
				authenticated = true
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(jsonRPCResponse{
					JSONRPC: "2.0",
					Error:   &rpcError{Code: -32001, Message: localizer.T(i18n.ErrUnauthorizedInvalidToken)},
				})
				return
			}
		}
	}

	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(jsonRPCResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32700, Message: localizer.T(i18n.ErrParseError)},
		})
		return
	}

	// Session validation: skip for initialize, require for everything else
	// once sessions are in use.
	if req.Method != "initialize" {
		sessionID := r.Header.Get("Mcp-Session-Id")
		if sessionID == "" {
			// Check if any sessions exist; if so, a session ID is required.
			hasSession := false
			s.sessions.Range(func(_, _ interface{}) bool {
				hasSession = true
				return false // stop iteration
			})
			if hasSession {
				http.Error(w, localizer.T(i18n.ErrSessionRequired), http.StatusBadRequest)
				return
			}
		} else {
			if _, ok := s.sessions.Load(sessionID); !ok {
				http.Error(w, localizer.T(i18n.ErrSessionNotFound), http.StatusNotFound)
				return
			}
		}
	}

	// Notifications: method present but no id.
	if isNotification(req) {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp := s.dispatch(req, authenticated, localizer)

	// If this is an initialize response, generate and attach session ID.
	if req.Method == "initialize" && resp.Error == nil {
		sid := generateSessionID()
		s.sessions.Store(sid, struct{}{})
		w.Header().Set("Mcp-Session-Id", sid)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// dispatch routes a JSON-RPC request to the appropriate handler.
// authenticated indicates whether the caller provided a valid Bearer token.
// l provides localized translations for user-facing messages.
func (s *Server) dispatch(req jsonRPCRequest, authenticated bool, l *i18n.Localizer) jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req, authenticated, l)
	case "tools/call":
		return s.handleToolsCall(req, authenticated, l)
	case "resources/list":
		return s.handleResourcesList(req, l)
	case "resources/read":
		return s.handleResourcesRead(req, l)
	default:
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: l.Tf(i18n.ErrMethodNotFound, req.Method)},
		}
	}
}

// serverInfo is returned in the initialize response.
type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// initializeResult is the full response body for the initialize method.
type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	ServerInfo      serverInfo     `json:"serverInfo"`
	Capabilities    map[string]any `json:"capabilities"`
}

// toolDefinition describes a single MCP tool.
type toolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// handleInitialize returns server info and the list of available tools.
func (s *Server) handleInitialize(req jsonRPCRequest) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: initializeResult{
			ProtocolVersion: "2025-11-25",
			ServerInfo:      serverInfo{Name: "a2a-sentinel", Version: "0.2.0"},
			Capabilities: map[string]any{
				"tools":     map[string]any{},
				"resources": map[string]any{},
			},
		},
	}
}

// toolsList returns the fixed list of tools exposed by the MCP server.
// l provides localized translations for tool and parameter descriptions.
func toolsList(l *i18n.Localizer) []toolDefinition {
	return []toolDefinition{
		// Read tools
		{
			Name:        "list_agents",
			Description: l.T(i18n.ToolListAgentsDesc),
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "health_check",
			Description: l.T(i18n.ToolHealthCheckDesc),
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "get_blocked_requests",
			Description: l.T(i18n.ToolGetBlockedRequestsDesc),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"since": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamSinceDesc),
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": l.T(i18n.ParamLimitDesc),
					},
				},
			},
		},
		{
			Name:        "get_agent_card",
			Description: l.T(i18n.ToolGetAgentCardDesc),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_name": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamAgentNameCardDesc),
					},
				},
				"required": []string{"agent_name"},
			},
		},
		{
			Name:        "get_aggregated_card",
			Description: l.T(i18n.ToolGetAggregatedCardDesc),
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "get_rate_limit_status",
			Description: l.T(i18n.ToolGetRateLimitStatusDesc),
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		// Write tools
		{
			Name:        "update_rate_limit",
			Description: l.T(i18n.ToolUpdateRateLimitDesc),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_name": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamAgentNameDesc),
					},
					"per_minute": map[string]any{
						"type":        "integer",
						"description": l.T(i18n.ParamPerMinuteDesc),
					},
				},
				"required": []string{"agent_name", "per_minute"},
			},
		},
		{
			Name:        "register_agent",
			Description: l.T(i18n.ToolRegisterAgentDesc),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamNameDesc),
					},
					"url": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamURLDesc),
					},
					"default": map[string]any{
						"type":        "boolean",
						"description": l.T(i18n.ParamDefaultDesc),
					},
				},
				"required": []string{"name", "url"},
			},
		},
		{
			Name:        "deregister_agent",
			Description: l.T(i18n.ToolDeregisterAgentDesc),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamNameDesc),
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "send_test_message",
			Description: l.T(i18n.ToolSendTestMessageDesc),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_name": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamAgentNameDesc),
					},
					"text": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamTextDesc),
					},
				},
				"required": []string{"agent_name", "text"},
			},
		},
		// Policy tools
		{
			Name:        "list_policies",
			Description: l.T(i18n.ToolListPoliciesDesc),
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "evaluate_policy",
			Description: l.T(i18n.ToolEvaluatePolicyDesc),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamAgentDesc),
					},
					"method": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamMethodDesc),
					},
					"user": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamUserDesc),
					},
					"ip": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamIPDesc),
					},
				},
			},
		},
		// Card change approval tools
		{
			Name:        "list_pending_changes",
			Description: l.T(i18n.ToolListPendingChangesDesc),
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "approve_card_change",
			Description: l.T(i18n.ToolApproveCardChangeDesc),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_name": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamAgentNameCardDesc),
					},
				},
				"required": []string{"agent_name"},
			},
		},
		{
			Name:        "reject_card_change",
			Description: l.T(i18n.ToolRejectCardChangeDesc),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_name": map[string]any{
						"type":        "string",
						"description": l.T(i18n.ParamAgentNameCardDesc),
					},
				},
				"required": []string{"agent_name"},
			},
		},
	}
}

// handleToolsList returns the list of available tools.
// Anonymous sessions only see read tools; authenticated sessions see all tools.
func (s *Server) handleToolsList(req jsonRPCRequest, authenticated bool, l *i18n.Localizer) jsonRPCResponse {
	all := toolsList(l)
	if authenticated || s.token == "" {
		// Authenticated or no token configured: return all tools.
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": all},
		}
	}
	// Anonymous with token configured: filter to read-only tools.
	var filtered []toolDefinition
	for _, t := range all {
		if !isWriteTool(t.Name) {
			filtered = append(filtered, t)
		}
	}
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"tools": filtered},
	}
}

// ── resources ────────────────────────────────────────────────────────────────

// resourceDefinition describes a single MCP resource.
type resourceDefinition struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

// resourceReadParams holds the params for a resources/read request.
type resourceReadParams struct {
	URI string `json:"uri"`
}

// resourceContent is a single content item within a resource read result.
type resourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

// resourcesList returns the fixed list of resources exposed by the MCP server.
// l provides localized translations for resource names and descriptions.
func resourcesList(l *i18n.Localizer) []resourceDefinition {
	return []resourceDefinition{
		{
			URI:         "sentinel://config",
			Name:        l.T(i18n.ResConfigName),
			Description: l.T(i18n.ResConfigDesc),
			MimeType:    "application/json",
		},
		{
			URI:         "sentinel://metrics",
			Name:        l.T(i18n.ResMetricsName),
			Description: l.T(i18n.ResMetricsDesc),
			MimeType:    "application/json",
		},
		{
			URI:         "sentinel://agents/{name}",
			Name:        l.T(i18n.ResAgentDetailName),
			Description: l.T(i18n.ResAgentDetailDesc),
			MimeType:    "application/json",
		},
		{
			URI:         "sentinel://security/report",
			Name:        l.T(i18n.ResSecurityName),
			Description: l.T(i18n.ResSecurityDesc),
			MimeType:    "application/json",
		},
	}
}

// handleResourcesList returns the list of available resources.
func (s *Server) handleResourcesList(req jsonRPCRequest, l *i18n.Localizer) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"resources": resourcesList(l)},
	}
}

// handleResourcesRead reads a specific resource by URI.
func (s *Server) handleResourcesRead(req jsonRPCRequest, l *i18n.Localizer) jsonRPCResponse {
	var params resourceReadParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: l.Tf(i18n.ErrInvalidParams, err.Error())},
		}
	}

	var data interface{}
	var err error

	switch {
	case params.URI == "sentinel://config":
		data = s.bridge.GetConfig()
	case params.URI == "sentinel://metrics":
		data = s.bridge.GetMetrics()
	case params.URI == "sentinel://security/report":
		data = s.bridge.GetSecurityReport()
	case strings.HasPrefix(params.URI, "sentinel://agents/"):
		agentName := strings.TrimPrefix(params.URI, "sentinel://agents/")
		if agentName == "" || agentName == "{name}" {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &rpcError{Code: -32602, Message: l.T(i18n.ErrAgentNameRequiredInURI)},
			}
		}
		data, err = s.bridge.GetAgentCard(agentName)
		if err != nil {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &rpcError{Code: -32603, Message: l.Tf(i18n.ErrInternalError, err.Error())},
			}
		}
	default:
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: l.Tf(i18n.ErrUnknownResourceURI, params.URI)},
		}
	}

	b, err := json.Marshal(data)
	if err != nil {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32603, Message: l.Tf(i18n.ErrInternalError, err.Error())},
		}
	}

	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"contents": []resourceContent{
				{
					URI:      params.URI,
					MimeType: "application/json",
					Text:     string(b),
				},
			},
		},
	}
}
