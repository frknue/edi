// Command edi-mcp is a Model Context Protocol (MCP) server that exposes the
// Life RPG agent tools to an AI client (Claude Desktop, Claude Code, etc.) over
// stdio. It is a thin proxy: tools/list and tools/call forward to the running
// server's /api/agent/tools registry — the SAME service path the web UI and CLI
// use. The agent therefore has no privileged or hidden data access.
//
// Transport: newline-delimited JSON-RPC 2.0 on stdin/stdout (the MCP stdio
// convention). All logging goes to stderr so stdout stays a clean protocol stream.
//
// Configure the target server with EDI_API (default http://localhost:8080).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"edi/internal/apiclient"
)

const (
	serverName      = "edi-mcp"
	serverVersion   = "0.1.0"
	protocolVersion = "2024-11-05"
)

// JSON-RPC 2.0 message shapes.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // absent => notification
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	codeMethodNotFound = -32601
	codeInternalError  = -32603
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetPrefix("[edi-mcp] ")
	addr := envOr("EDI_API", "http://localhost:8080")
	client := apiclient.New(addr)
	client.Token = os.Getenv("EDI_TOKEN") // optional bearer auth (server started with EDI_TOKEN)
	log.Printf("starting; proxying tools to %s (auth: %v)", addr, client.Token != "")

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // allow large messages
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("bad message: %v", err)
			continue
		}

		resp, respond := dispatch(client, req)
		if !respond { // notification — no reply
			continue
		}
		if err := enc.Encode(resp); err != nil { // Encode appends a newline
			log.Printf("write error: %v", err)
			return
		}
		_ = out.Flush()
	}
	if err := in.Err(); err != nil {
		log.Printf("stdin error: %v", err)
	}
}

// dispatch handles one request. The second return value is false for
// notifications (no id), which must not produce a response.
func dispatch(client *apiclient.Client, req rpcRequest) (rpcResponse, bool) {
	isNotification := len(req.ID) == 0
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": serverName, "version": serverVersion},
		}
	case "notifications/initialized", "notifications/cancelled":
		return resp, false // notifications: ignore, no reply
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		result, err := listTools(client)
		if err != nil {
			resp.Error = &rpcError{Code: codeInternalError, Message: err.Error()}
		} else {
			resp.Result = result
		}
	case "tools/call":
		resp.Result = callTool(client, req.Params)
	default:
		if isNotification {
			return resp, false
		}
		resp.Error = &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}
	}

	if isNotification {
		return resp, false
	}
	return resp, true
}

// mcpTool is one entry in a tools/list result (note: inputSchema, camelCase).
type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func listTools(client *apiclient.Client) (map[string]any, error) {
	specs, err := client.ListTools()
	if err != nil {
		return nil, err
	}
	tools := make([]mcpTool, 0, len(specs))
	for _, s := range specs {
		schema := s.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		tools = append(tools, mcpTool{Name: s.Name, Description: s.Description, InputSchema: schema})
	}
	return map[string]any{"tools": tools}, nil
}

// callTool invokes a tool. Per MCP, tool *execution* failures are returned as a
// successful result with isError=true (not a JSON-RPC protocol error), so the
// model can read and react to them.
func callTool(client *apiclient.Client, params json.RawMessage) map[string]any {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return errorResult(fmt.Sprintf("invalid params: %v", err))
	}
	if p.Name == "" {
		return errorResult("missing tool name")
	}
	result, err := client.InvokeTool(p.Name, p.Arguments)
	if err != nil {
		return errorResult(err.Error())
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(result)}},
	}
}

func errorResult(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": msg}},
		"isError": true,
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
