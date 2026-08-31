// Package mcp implements a thin stdio MCP server over the local gadak mirror.
//
// It is the access path for clients without a shell (Claude Desktop and similar).
// Agents that can run commands should prefer `gadak sql` / `gadak issue` / `gadak search`
// instead — see docs/MCP.md and contracts/agent.md.
//
// Protocol surface is deliberately small and implemented with the Go stdlib only:
// initialize, tools/list, tools/call, notifications/initialized, ping.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/midagedev/gadak/internal/store"
)

// ProtocolVersion is the MCP protocol version this server speaks. When a client
// requests a different version we still answer with this one and do not reject
// the session (MCP version negotiation).
const ProtocolVersion = "2025-03-26"

// JSON-RPC 2.0 error codes used by the protocol layer.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// Server is a stdio MCP session bound to one mirror database.
type Server struct {
	DBPath  string
	Profile string
	// Version is the gadak binary version reported in serverInfo.
	Version string

	// resultByteCap overrides the JSON payload cap. Zero means maxResultBytes
	// (256KiB). Tests lower it; production must stay at the default.
	resultByteCap int

	mu sync.Mutex
	db *store.DB
}

// New constructs a server. The database is opened on first tool call so
// initialize / tools/list work even when the mirror is missing (the tool then
// returns a clear isError message).
func New(dbPath, profile, version string) *Server {
	return &Server{DBPath: dbPath, Profile: profile, Version: version}
}

// Serve reads newline-delimited JSON-RPC messages from in and writes responses
// to out. Logs must never go to out — callers should leave log output on stderr.
// Returns when in is exhausted or a fatal I/O error occurs.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	// Every exit path releases the mirror — a failed Encode or scanner error
	// used to leak the handle for the process lifetime.
	defer s.closeDB()
	sc := bufio.NewScanner(in)
	// SQL and large keys can exceed the default 64 KiB token; 4 MiB is plenty
	// for a single MCP request on this surface.
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)

	enc := json.NewEncoder(out)
	// Compact one-line frames: MCP stdio forbids embedded newlines in a message.
	enc.SetEscapeHTML(false)

	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		resp := s.handleLine(line)
		if resp == nil {
			continue // notification
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return nil
}

// handleLine parses one frame and returns a response, or nil for notifications.
func (s *Server) handleLine(line []byte) *rpcResponse {
	var msg rpcRequest
	if err := json.Unmarshal(line, &msg); err != nil {
		return errResponse(nil, codeParseError, "parse error: "+err.Error())
	}
	if msg.JSONRPC != "" && msg.JSONRPC != "2.0" {
		return errResponse(msg.ID, codeInvalidRequest, "jsonrpc must be \"2.0\"")
	}
	// Notifications have no id (or null) and no response.
	if isNotification(msg) {
		s.handleNotification(msg.Method)
		return nil
	}
	if msg.Method == "" {
		return errResponse(msg.ID, codeInvalidRequest, "missing method")
	}
	return s.dispatch(msg)
}

func isNotification(msg rpcRequest) bool {
	if msg.Method == "" {
		return false
	}
	// A request always has an id; a notification must not.
	if len(msg.ID) == 0 || string(msg.ID) == "null" {
		// Heuristic: methods under notifications/ are always notifications.
		// Also treat missing id as notification per JSON-RPC.
		return true
	}
	return false
}

func (s *Server) handleNotification(method string) {
	switch method {
	case "notifications/initialized", "initialized":
		// No-op: session is stateless beyond the DB.
	default:
		// Unknown notifications are ignored (forward-compatible).
	}
}

func (s *Server) dispatch(msg rpcRequest) *rpcResponse {
	switch msg.Method {
	case "initialize":
		return s.handleInitialize(msg)
	case "tools/list":
		return s.handleToolsList(msg)
	case "tools/call":
		return s.handleToolsCall(msg)
	case "ping":
		return okResponse(msg.ID, map[string]any{})
	default:
		return errResponse(msg.ID, codeMethodNotFound, "method not found: "+msg.Method)
	}
}

func (s *Server) handleInitialize(msg rpcRequest) *rpcResponse {
	// Always speak our version; do not reject a client that asked for another.
	version := s.Version
	if version == "" {
		version = "dev"
	}
	result := map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "gadak",
			"version": version,
		},
		"instructions": `You are querying a local SQLite mirror of this machine's Jira and Confluence.
Reads are free and offline. You cannot write to Jira or to this database
through these tools.

Pick the tool by the shape of the question:
- gadak_query is the default. Anything countable, grouped, joined,
  historical, or derived (reopen_count, status_changed_at, epic_key;
  days-in-status is computed from status_changed_at, not a column).
  SQLite SELECT or WITH only. Prefer the issues_full view when you
  need a title. Filter on status_category
  (new|inprogress|done) or status_id / issue_type_id — never
  status = 'In Progress' or other display names. Those are localized
  per Jira account and return zero rows with no error.
- gadak_search only for recalled wording when you do not have keys.
  Argument name is query (string). Aliases: text, q.
- gadak_issue when you have one key and need comments, history, links.
- gadak_status before any answer you will act on. Read last_error and
  watermark.
- gadak_show to put keys on the human's running gadak window. It
  returns no issue rows. After you have keys, call show; do not paste
  a markdown table.

If a tool returns a message starting with ERROR:, that is not "no
rows" — retry with the field names it lists. Empty rows after a
status-name WHERE usually mean a localized name: rewrite using
status_category and retry. A result with truncation_reason is
incomplete; tighten LIMIT or columns. Do not treat a truncated page
as the full set.`,
	}
	return okResponse(msg.ID, result)
}

func (s *Server) handleToolsList(msg rpcRequest) *rpcResponse {
	return okResponse(msg.ID, map[string]any{"tools": toolDefinitions()})
}

func (s *Server) handleToolsCall(msg rpcRequest) *rpcResponse {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if len(msg.Params) > 0 {
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errResponse(msg.ID, codeInvalidParams, "invalid tools/call params: "+err.Error())
		}
	}
	if params.Name == "" {
		return errResponse(msg.ID, codeInvalidParams, "tools/call requires params.name")
	}
	// Unknown tool names are protocol errors so clients can surface them cleanly.
	switch params.Name {
	case toolQuery, toolSearch, toolIssue, toolStatus, toolShow:
	default:
		return errResponse(msg.ID, codeInvalidParams, "unknown tool: "+params.Name)
	}
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}
	content, isErr := s.callTool(params.Name, params.Arguments)
	return okResponse(msg.ID, callResult{Content: content, IsError: isErr})
}

// ensureDB opens the store on first use. Missing files become tool errors with
// setup guidance rather than a process exit, so the MCP session can still list tools.
func (s *Server) ensureDB() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return nil
	}
	if msg := dbMissingMessage(s.DBPath); msg != "" {
		return fmt.Errorf("%s", msg)
	}
	db, err := store.Open(s.DBPath)
	if err != nil {
		return fmt.Errorf("open mirror: %w", err)
	}
	s.db = db
	return nil
}

func (s *Server) closeDB() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
}

// rpcRequest is a JSON-RPC 2.0 request or notification.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// rpcResponse is a JSON-RPC 2.0 response.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

func okResponse(id json.RawMessage, result any) *rpcResponse {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return &rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func errResponse(id json.RawMessage, code int, message string) *rpcResponse {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return &rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	}
}

// Logf writes a diagnostic line to stderr. Never use the process logger against
// stdout while serving MCP — that corrupts the JSON-RPC stream.
func Logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gadak mcp: "+format+"\n", args...)
}
