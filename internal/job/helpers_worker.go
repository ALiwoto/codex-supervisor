package job

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ALiwoto/codex-supervisor/internal/processTree"
)

func RunWorker(manager Manager, id string) error {
	directory, err := manager.Directory(id)
	if err != nil {
		return err
	}

	var request Request
	if err = readJSON(filepath.Join(directory, "request.json"), &request); err != nil {
		return err
	}

	current := &worker{
		snapshot: Snapshot{
			ID:      id,
			State:   "starting",
			Started: time.Now().UTC(),
			LogPath: filepath.Join(directory, "output.log"),
		},
		changed: make(chan struct{}),
		cancel:  make(chan struct{}),
	}
	// Every startup failure is a terminal receipt, including failure to create the command.
	defer func() {
		current.mu.Lock()
		defer current.mu.Unlock()
		if !terminal(current.snapshot.State) {
			current.snapshot.State = "failed"
			current.snapshot.Error = "worker stopped before completion; see worker.log"
			now := time.Now().UTC()
			current.snapshot.Finished = &now
			if saveError := writeJSON(filepath.Join(directory, "status.json"), current.snapshot); saveError != nil {
				fmt.Fprintln(os.Stderr, saveError)
			}
		}
	}()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}

	defer listener.Close()
	current.connection = Connection{
		Address: "http://" + listener.Addr().String(),
		Token:   randomID(),
	}
	if err = writeJSON(filepath.Join(directory, "connection.json"), current.connection); err != nil {
		return err
	}

	output, err := os.OpenFile(current.snapshot.LogPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return err
	}

	writer := &logWriter{
		file: output,
		path: current.snapshot.LogPath,
	}
	defer func() {
		writer.file.Close()
	}()
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		return err
	}

	defer readPipe.Close()
	defer writePipe.Close()
	// Each worker owns one job: resolve executables using that job's cwd and PATH.
	if err = os.Chdir(request.Directory); err != nil {
		return err
	}

	for name, value := range request.Environment {
		if err = os.Setenv(name, value); err != nil {
			return err
		}
	}
	process, err := processTree.Start(
		request.Command,
		request.Directory,
		os.Environ(),
		writePipe,
	)
	if err != nil {
		current.snapshot.State = "failed"
		current.snapshot.Error = err.Error()
		now := time.Now().UTC()
		current.snapshot.Finished = &now
		return writeJSON(filepath.Join(directory, "status.json"), current.snapshot)
	}

	defer process.Close()
	writePipe.Close()
	current.snapshot.PID = process.PID
	current.snapshot.State = "running"
	if err = writeJSON(filepath.Join(directory, "status.json"), current.snapshot); err != nil {
		return err
	}

	server := &http.Server{
		Handler:           http.HandlerFunc(current.HandleRequest),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(listener)
	}()
	defer func() {
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if shutdownError := server.Shutdown(ctx); shutdownError != nil {
			server.Close()
		}
	}()
	logDone := make(chan error, 1)
	go func() {
		_, copyError := io.Copy(writer, readPipe)
		logDone <- copyError
	}()
	finished := make(chan completion, 1)
	go func() {
		code, waitError := process.Wait()
		finished <- completion{
			code: code,
			err:  waitError,
		}
	}()
	result := monitor(current, process, writer, request, finished, logDone, serverErrors)
	current.mu.Lock()
	current.snapshot.State = result.state
	current.snapshot.ExitCode = &result.code
	current.snapshot.Error = result.message
	now := time.Now().UTC()
	current.snapshot.Finished = &now
	current.snapshot.ElapsedSeconds = now.Sub(current.snapshot.Started).Seconds()
	_, rotated := writer.progress()
	current.snapshot.LogTruncated = rotated
	if rotated {
		current.snapshot.PreviousLogPath = writer.path + ".1"
	}

	if result.state != "succeeded" {
		current.snapshot.Diagnostics = diagnostics(writer.path)
	}
	err = writeJSON(filepath.Join(directory, "status.json"), current.snapshot)
	close(current.changed)
	current.changed = make(chan struct{})
	current.mu.Unlock()
	return err
}

func monitor(current *worker, process *processTree.Process, writer *logWriter, request Request,
	finished <-chan completion, logDone <-chan error, serverErrors <-chan error) outcome {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	started := time.Now()
	lastActivity := started
	var previous processTree.Usage
	var previousOutput int64
	var previousHeartbeat time.Time
	var rootDone *completion
	logsFinished := false
	state := "succeeded"
	message := ""
	cancel := current.cancel
	stopTree := func(reason, detail string) {
		if state != "succeeded" {
			return
		}
		state = reason
		message = detail
		if err := process.Kill(); err != nil {
			message += "; termination failed: " + err.Error()
		}
	}
	for {
		select {
		case result := <-finished:
			rootDone = &result
			finished = nil
			now := time.Now().UTC()
			current.mu.Lock()
			current.snapshot.CommandFinished = &now
			current.mu.Unlock()
			if !request.WaitForChildren {
				usage, err := process.Usage()
				if err == nil {
					current.mu.Lock()
					current.snapshot.ProcessesAtCommandExit = usage.Processes
					current.mu.Unlock()
				}

				// A foreground command owns completion. Compiler servers and other
				// inherited helpers must not prolong the job or hold its output pipe.
				if err = process.Kill(); err != nil {
					stopTree("failed", "clean up command descendants: "+err.Error())
				}
			}
		case err := <-logDone:
			logsFinished = true
			logDone = nil
			if err != nil {
				stopTree("failed", "capture output: "+err.Error())
			}
		case <-cancel:
			cancel = nil
			stopTree("cancelled", "cancelled explicitly")
		case err := <-serverErrors:
			serverErrors = nil
			if !errors.Is(err, http.ErrServerClosed) {
				stopTree("failed", "worker listener stopped: "+err.Error())
			}
		case <-ticker.C:
		}
		now := time.Now()
		usage, usageError := process.Usage()
		outputBytes, rotated := writer.progress()
		if usageError == nil &&
			(usage.CPUSeconds-previous.CPUSeconds > 0.02 || usage.IOBytes != previous.IOBytes ||
				usage.Processes != previous.Processes) {
			lastActivity = now
		}

		if outputBytes != previousOutput {
			lastActivity = now
		}

		if request.Heartbeat != "" {
			if info, err := os.Stat(request.Heartbeat); err == nil &&
				info.ModTime() != previousHeartbeat {
				previousHeartbeat = info.ModTime()
				lastActivity = now
			}
		}

		if request.TimeoutSeconds > 0 &&
			now.Sub(started) >= time.Duration(request.TimeoutSeconds)*time.Second {
			stopTree("timed_out", "explicit job deadline exceeded")
		}
		current.mu.Lock()
		current.snapshot.ElapsedSeconds = now.Sub(current.snapshot.Started).Seconds()
		current.snapshot.LogTruncated = rotated
		warning := ""
		if usageError != nil {
			warning = "process activity monitoring unavailable: " + usageError.Error()
		} else {
			current.snapshot.Usage = usage
			if request.StallSeconds > 0 &&
				now.Sub(lastActivity) >= time.Duration(request.StallSeconds)*time.Second {
				warning = fmt.Sprintf(
					"suspected stall: no observed CPU, I/O, output or heartbeat progress for %ds; job remains running",
					request.StallSeconds,
				)
			}
		}

		if warning != current.snapshot.Warning {
			current.snapshot.Warning = warning
			close(current.changed)
			current.changed = make(chan struct{})
		}
		current.mu.Unlock()
		previous = usage
		previousOutput = outputBytes
		if rootDone != nil && logsFinished && (usageError != nil || usage.Processes == 0) {
			if rootDone.err != nil {
				state = "failed"
				message = rootDone.err.Error()
			} else if state == "succeeded" && rootDone.code != 0 {
				state = "failed"
			}

			return outcome{
				state:   state,
				code:    rootDone.code,
				message: message,
			}
		}
	}
}
