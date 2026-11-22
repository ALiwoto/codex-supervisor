package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ALiwoto/codex-supervisor/internal/job"
)

func (current *server) send(value response) {
	current.writeMu.Lock()
	defer current.writeMu.Unlock()
	_ = json.NewEncoder(current.output).Encode(value)
}

func (current *server) sendError(id json.RawMessage, code int, message string) {
	current.send(response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: message,
		},
	})
}

func (current *server) initialize(request message) {
	var parameters struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(request.Params, &parameters); err != nil ||
		parameters.ProtocolVersion == "" {
		current.sendError(request.ID, -32602, "protocolVersion is required")
		return
	}

	version := parameters.ProtocolVersion
	if version != "2024-11-05" && version != "2025-03-26" && version != "2025-06-18" &&
		version != "2025-11-25" {
		version = "2025-11-25"
	}
	current.initialized = true
	current.send(response{
		JSONRPC: "2.0",
		ID:      request.ID,
		Result: initializeResult{
			ProtocolVersion: version,
			ServerInfo: serverInfo{
				Name:    "codex-supervisor",
				Version: "0.1.0",
			},
			Instructions: instructions,
		},
	})
}

func (current *server) cancelRequest(parameters json.RawMessage) {
	var request struct {
		RequestID json.RawMessage `json:"requestId"`
	}
	if json.Unmarshal(parameters, &request) != nil {
		return
	}
	current.requestsMu.Lock()
	defer current.requestsMu.Unlock()
	if cancel := current.requests[string(request.RequestID)]; cancel != nil {
		cancel()
	}
}

func (current *server) launch(parent context.Context, request message) {
	id := string(request.ID)
	ctx, cancel := context.WithCancel(parent)
	current.requestsMu.Lock()
	if _, exists := current.requests[id]; exists {
		current.requestsMu.Unlock()
		cancel()
		current.sendError(request.ID, -32600, "duplicate active request id")
		return
	}
	current.requests[id] = cancel
	current.requestsMu.Unlock()
	current.group.Add(1)
	go func() {
		defer current.group.Done()
		defer cancel()
		defer func() {
			current.requestsMu.Lock()
			delete(current.requests, id)
			current.requestsMu.Unlock()
		}()
		var parameters call
		if err := decode(request.Params, &parameters); err != nil {
			current.sendError(request.ID, -32602, err.Error())
			return
		}

		snapshot, err := current.call(ctx, parameters)
		if ctx.Err() == nil {
			current.send(response{
				JSONRPC: "2.0",
				ID:      request.ID,
				Result:  resultFor(snapshot, err),
			})
		}
	}()
}

func (current *server) call(ctx context.Context, parameters call) (job.Snapshot, error) {
	if parameters.Name == "start" || parameters.Name == "run" {
		request := job.Request{
			StallSeconds: 300,
		}
		if err := decode(parameters.Arguments, &request); err != nil {
			return job.Snapshot{}, err
		}

		if request.ID == "" {
			return job.Snapshot{}, fmt.Errorf(
				"id is required so an interrupted submission can be retried without duplicating the job",
			)
		}

		snapshot, err := current.manager.Start(ctx, request)
		if err != nil || parameters.Name == "start" {
			return snapshot, err
		}

		return current.manager.Wait(ctx, snapshot.ID, false)
	}

	var arguments jobArguments
	if err := decode(parameters.Arguments, &arguments); err != nil {
		return job.Snapshot{}, err
	}

	switch parameters.Name {
	case "wait":
		return current.manager.Wait(ctx, arguments.ID, arguments.AfterWarning)
	case "inspect":
		return current.manager.Inspect(ctx, arguments.ID)
	case "cancel":
		return current.manager.Cancel(ctx, arguments.ID)
	default:
		return job.Snapshot{}, fmt.Errorf("unknown tool %q", parameters.Name)
	}
}
