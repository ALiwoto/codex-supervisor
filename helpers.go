package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/ALiwoto/codex-supervisor/internal/job"
	"github.com/ALiwoto/codex-supervisor/internal/mcp"
)

func run(arguments []string) error {
	if len(arguments) == 0 || arguments[0] == "help" || arguments[0] == "--help" {
		fmt.Println(
			"codex-supervisor <mcp|run|start|wait|inspect|cancel> [--root DIR] [--id ID] [--request FILE|-] [--after-warning]\n\nrun/start read a JSON request; wait blocks without polling. Job receipts and bounded logs persist under the user cache directory. Ctrl+C detaches; use cancel to terminate a job. Commands run with this user's permissions, without a sandbox.",
		)
		return nil
	}

	root, err := job.DefaultRoot()
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet(arguments[0], flag.ContinueOnError)
	flags.StringVar(&root, "root", root, "persistent job directory")
	id := flags.String("id", "", "job id")
	requestFile := flags.String("request", "-", "JSON request file, or - for stdin")
	afterWarning := flags.Bool(
		"after-warning",
		false,
		"acknowledge the current warning and continue waiting",
	)
	if err = flags.Parse(arguments[1:]); err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}

	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}

	manager := job.Manager{
		Root: root,
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	var snapshot job.Snapshot
	switch arguments[0] {
	case "mcp":
		return mcp.Serve(ctx, os.Stdin, os.Stdout, manager)
	case "worker":
		return job.RunWorker(manager, *id)
	case "start", "run":
		request := job.Request{
			StallSeconds: 300,
		}
		var reader io.Reader = os.Stdin
		if *requestFile != "-" {
			file, openError := os.Open(*requestFile)
			if openError != nil {
				return openError
			}
			defer file.Close()
			reader = file
		}

		decoder := json.NewDecoder(io.LimitReader(reader, 1024*1024))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&request); err != nil {
			return err
		}

		if *id != "" {
			request.ID = *id
		}
		snapshot, err = manager.Start(ctx, request)
		if err == nil && arguments[0] == "run" {
			snapshot, err = manager.Wait(ctx, snapshot.ID, false)
		}
	case "wait":
		snapshot, err = manager.Wait(ctx, *id, *afterWarning)
	case "inspect":
		snapshot, err = manager.Inspect(ctx, *id)
	case "cancel":
		snapshot, err = manager.Cancel(ctx, *id)
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
	if err != nil {
		return err
	}

	if err = json.NewEncoder(os.Stdout).Encode(snapshot); err != nil {
		return err
	}

	if snapshot.State == "failed" || snapshot.State == "cancelled" || snapshot.State == "timed_out" {
		return fmt.Errorf("job %s: %s", snapshot.ID, snapshot.State)
	}

	return nil
}
