package mcp

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/ALiwoto/codex-supervisor/internal/job"
)

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content           []textContent `json:"content"`
	StructuredContent *job.Snapshot `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError,omitempty"`
}

type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type toolList struct {
	Tools []tool `json:"tools"`
}

type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    capabilities `json:"capabilities"`
	ServerInfo      serverInfo   `json:"serverInfo"`
	Instructions    string       `json:"instructions"`
}

type capabilities struct {
	Tools struct{} `json:"tools"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type server struct {
	manager     job.Manager
	output      io.Writer
	writeMu     sync.Mutex
	requestsMu  sync.Mutex
	requests    map[string]context.CancelFunc
	group       sync.WaitGroup
	initialized bool
}

type call struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type jobArguments struct {
	ID           string `json:"id"`
	AfterWarning bool   `json:"after_warning,omitempty"`
}
