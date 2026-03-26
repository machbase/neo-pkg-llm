package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"neo-pkg-llm/logger"
	"neo-pkg-llm/tools"
)

// JSON-RPC 2.0 types
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *jsonrpcError `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP protocol types
type mcpInitializeResult struct {
	ProtocolVersion string            `json:"protocolVersion"`
	Capabilities    mcpCapabilities   `json:"capabilities"`
	ServerInfo      mcpServerInfo     `json:"serverInfo"`
}

type mcpCapabilities struct {
	Tools *mcpToolsCap `json:"tools,omitempty"`
}

type mcpToolsCap struct {
	ListChanged bool `json:"listChanged"`
}

type mcpServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type mcpToolDef struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	InputSchema tools.ToolParameters `json:"inputSchema"`
}

type mcpToolsListResult struct {
	Tools []mcpToolDef `json:"tools"`
}

type mcpToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type mcpToolCallResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Server implements the MCP protocol over stdio.
type Server struct {
	registry *tools.Registry
	mu       sync.Mutex
}

func NewServer(registry *tools.Registry) *Server {
	return &Server{registry: registry}
}

// Run starts the MCP server reading from stdin and writing to stdout.
func (s *Server) Run() error {
	// logger already writes to stdout + file
	logger.Infof("[MCP] Machbase Neo MCP Server (Go) starting...")
	logger.Infof("[MCP] Tools: %d registered", len(s.registry.ToolNames()))

	reader := bufio.NewReader(os.Stdin)
	writer := os.Stdout

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				logger.Infof("[MCP] stdin closed, exiting")
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		var req jsonrpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			logger.Infof("[MCP] invalid JSON: %v", err)
			continue
		}

		resp := s.handleRequest(&req)
		if resp == nil {
			continue // notification, no response needed
		}

		data, _ := json.Marshal(resp)
		s.mu.Lock()
		writer.Write(data)
		writer.Write([]byte("\n"))
		s.mu.Unlock()
	}
}

func (s *Server) handleRequest(req *jsonrpcRequest) *jsonrpcResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "initialized":
		logger.Infof("[MCP] Client initialized")
		return nil // notification
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	case "ping":
		return &jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	default:
		logger.Infof("[MCP] Unknown method: %s", req.Method)
		return &jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonrpcError{Code: -32601, Message: "Method not found: " + req.Method},
		}
	}
}

func (s *Server) handleInitialize(req *jsonrpcRequest) *jsonrpcResponse {
	logger.Infof("[MCP] Initialize request")
	return &jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: mcpInitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: mcpCapabilities{
				Tools: &mcpToolsCap{ListChanged: false},
			},
			ServerInfo: mcpServerInfo{
				Name:    "machbase-neo-go",
				Version: "1.0.0",
			},
		},
	}
}

func (s *Server) handleToolsList(req *jsonrpcRequest) *jsonrpcResponse {
	logger.Infof("[MCP] tools/list request")
	var toolDefs []mcpToolDef
	for _, name := range s.registry.ToolNames() {
		t := s.registry.Get(name)
		toolDefs = append(toolDefs, mcpToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}
	return &jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  mcpToolsListResult{Tools: toolDefs},
	}
}

func (s *Server) handleToolsCall(req *jsonrpcRequest) *jsonrpcResponse {
	var params mcpToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonrpcError{Code: -32602, Message: "Invalid params: " + err.Error()},
		}
	}

	logger.Infof("[MCP] tools/call: %s", params.Name)

	result, err := s.registry.ExecuteMap(params.Name, params.Arguments)
	if err != nil {
		return &jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mcpToolCallResult{
				Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}},
				IsError: true,
			},
		}
	}

	return &jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: mcpToolCallResult{
			Content: []mcpContent{{Type: "text", Text: result}},
		},
	}
}
