package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ALiwoto/codex-supervisor/internal/job"
)

func Serve(ctx context.Context, input io.Reader, output io.Writer, manager job.Manager) error {
	ctx, cancel := context.WithCancel(ctx)
	current := &server{
		manager:  manager,
		output:   output,
		requests: make(map[string]context.CancelFunc),
	}
	defer func() {
		cancel()
		current.group.Wait()
	}()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		var request message
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			current.sendError(json.RawMessage("null"), -32700, "invalid JSON")
			continue
		}

		if request.JSONRPC != "2.0" || request.Method == "" {
			current.sendError(json.RawMessage("null"), -32600, "invalid JSON-RPC request")
			continue
		}

		if len(request.ID) == 0 {
			if request.Method == "notifications/cancelled" {
				current.cancelRequest(request.Params)
			}
			continue
		}

		if request.Method == "initialize" {
			current.initialize(request)
			continue
		}

		if request.Method == "ping" {
			current.send(response{
				JSONRPC: "2.0",
				ID:      request.ID,
				Result:  struct{}{},
			})
			continue
		}

		if !current.initialized {
			current.sendError(request.ID, -32600, "initialize first")
			continue
		}
		switch request.Method {
		case "tools/list":
			current.send(
				response{
					JSONRPC: "2.0",
					ID:      request.ID,
					Result: toolList{
						Tools: []tool{
							{
								Name:        "run",
								Description: "Start a durable job and block until completion or suspected stall; returns bounded diagnostics. Prefer for long builds/exports. No polling needed.",
								InputSchema: json.RawMessage(requestSchema),
							},
							{
								Name:        "start",
								Description: "Start a durable job and return its ID immediately. Do independent work, then call wait once.",
								InputSchema: json.RawMessage(requestSchema),
							},
							{
								Name:        "wait",
								Description: "Block until this job finishes or reports a suspected stall. No periodic status returns. Cancelling this call leaves the job running.",
								InputSchema: json.RawMessage(waitSchema),
							},
							{
								Name:        "inspect",
								Description: "Read one current snapshot for diagnosis or recovery. Do not poll this tool for completion; use wait.",
								InputSchema: json.RawMessage(idSchema),
							},
							{
								Name:        "cancel",
								Description: "Explicitly terminate the entire job process tree and return its final receipt.",
								InputSchema: json.RawMessage(idSchema),
							},
						},
					},
				},
			)
		case "tools/call":
			current.launch(ctx, request)
		default:
			current.sendError(request.ID, -32601, "method not found")
		}
	}
	return scanner.Err()
}

func decode(data json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func resultFor(snapshot job.Snapshot, err error) toolResult {
	if err != nil {
		return toolResult{
			Content: []textContent{
				{
					Type: "text",
					Text: err.Error(),
				},
			},
			IsError: true,
		}
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return toolResult{
			Content: []textContent{
				{
					Type: "text",
					Text: fmt.Sprint(err),
				},
			},
			IsError: true,
		}
	}

	return toolResult{
		Content: []textContent{
			{
				Type: "text",
				Text: string(data),
			},
		},
		StructuredContent: &snapshot,
		IsError:           snapshot.State == "failed" || snapshot.State == "timed_out",
	}
}
