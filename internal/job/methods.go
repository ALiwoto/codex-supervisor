package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/ALiwoto/codex-supervisor/internal/processTree"
)

func (manager Manager) Directory(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("job id must contain 1–80 letters, digits, hyphens or underscores")
	}

	return filepath.Join(manager.Root, id), nil
}

func (manager Manager) Start(ctx context.Context, request Request) (Snapshot, error) {
	if len(request.Command) == 0 || request.Command[0] == "" {
		return Snapshot{}, fmt.Errorf("command must contain an executable and its arguments")
	}

	if !filepath.IsAbs(request.Directory) {
		return Snapshot{}, fmt.Errorf("directory must be an absolute path")
	}

	if request.TimeoutSeconds < 0 || request.StallSeconds < 0 || request.TimeoutSeconds > 31536000 ||
		request.StallSeconds > 31536000 {
		return Snapshot{}, fmt.Errorf("timeouts must be between 0 and 31536000 seconds")
	}

	if request.Heartbeat != "" && !filepath.IsAbs(request.Heartbeat) {
		return Snapshot{}, fmt.Errorf("heartbeat must be an absolute file path")
	}
	for _, argument := range request.Command {
		if strings.ContainsRune(argument, 0) {
			return Snapshot{}, fmt.Errorf("command arguments cannot contain NUL")
		}
	}
	for name, value := range request.Environment {
		if name == "" || strings.ContainsAny(name, "=\x00") || strings.ContainsRune(value, 0) {
			return Snapshot{}, fmt.Errorf("invalid environment variable name or value")
		}
	}
	info, err := os.Stat(request.Directory)
	if err != nil {
		return Snapshot{}, err
	}

	if !info.IsDir() {
		return Snapshot{}, fmt.Errorf("working directory is not a directory")
	}

	if request.ID == "" {
		request.ID = randomID()
	}

	directory, err := manager.Directory(request.ID)
	if err != nil {
		return Snapshot{}, err
	}

	if err = os.MkdirAll(manager.Root, 0700); err != nil {
		return Snapshot{}, err
	}

	err = os.Mkdir(directory, 0700)
	if errors.Is(err, os.ErrExist) {
		var previous Request
		if err = readJSON(filepath.Join(directory, "request.json"), &previous); err != nil {
			return Snapshot{}, fmt.Errorf(
				"job %s already reserved; inspect it before retrying: %w",
				request.ID,
				err,
			)
		}

		if !reflect.DeepEqual(previous, request) {
			return Snapshot{}, fmt.Errorf(
				"job %s already exists with a different request",
				request.ID,
			)
		}

		return manager.Inspect(ctx, request.ID)
	}

	if err != nil {
		return Snapshot{}, err
	}

	if err = writeJSON(filepath.Join(directory, "request.json"), request); err != nil {
		return Snapshot{}, err
	}

	executable, err := os.Executable()
	if err != nil {
		return Snapshot{}, err
	}

	workerLog, err := os.OpenFile(
		filepath.Join(directory, "worker.log"),
		os.O_CREATE|os.O_WRONLY|os.O_EXCL,
		0600,
	)
	if err != nil {
		return Snapshot{}, err
	}

	defer workerLog.Close()
	command := exec.Command(executable, "worker", "--root", manager.Root, "--id", request.ID)
	command.Stdout = workerLog
	command.Stderr = workerLog
	processTree.Detach(command)
	if err = command.Start(); err != nil {
		return Snapshot{}, err
	}

	_ = command.Process.Release()
	startup, stop := context.WithTimeout(ctx, 15*time.Second)
	defer stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var snapshot Snapshot
		if err = readJSON(filepath.Join(directory, "status.json"), &snapshot); err == nil {
			return snapshot, nil
		}
		select {
		case <-startup.Done():
			return Snapshot{}, fmt.Errorf(
				"job %s startup not confirmed; inspect worker.log; do not resubmit under another id: %w",
				request.ID,
				startup.Err(),
			)
		case <-ticker.C:
		}
	}
}

func (manager Manager) Inspect(ctx context.Context, id string) (Snapshot, error) {
	return manager.query(ctx, id, "status")
}

func (manager Manager) Wait(ctx context.Context, id string, afterWarning bool) (Snapshot, error) {
	action := "wait"
	if afterWarning {
		action += "?after_warning=true"
	}

	return manager.query(ctx, id, action)
}

func (manager Manager) Cancel(ctx context.Context, id string) (Snapshot, error) {
	return manager.query(ctx, id, "cancel")
}

func (manager Manager) query(ctx context.Context, id string, action string) (Snapshot, error) {
	directory, err := manager.Directory(id)
	if err != nil {
		return Snapshot{}, err
	}

	var snapshot Snapshot
	statusPath := filepath.Join(directory, "status.json")
	if err = readJSON(statusPath, &snapshot); err != nil {
		return Snapshot{}, err
	}

	if terminal(snapshot.State) {
		return snapshot, nil
	}

	var connection Connection
	if err = readJSON(filepath.Join(directory, "connection.json"), &connection); err != nil {
		return Snapshot{}, err
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 3 * time.Second,
		}).DialContext,
	}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		connection.Address+"/"+action,
		nil,
	)
	if err != nil {
		return Snapshot{}, err
	}

	request.Header.Set("Authorization", "Bearer "+connection.Token)
	response, err := (&http.Client{
		Transport: transport,
	}).Do(request)
	if err != nil {
		// Completion may close the listener between reading the receipt and dialing.
		if readJSON(statusPath, &snapshot) == nil && terminal(snapshot.State) {
			return snapshot, nil
		}

		return Snapshot{}, fmt.Errorf(
			"job %s worker unavailable; receipt is not proof of completion (%s): %w",
			id,
			directory,
			err,
		)
	}

	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("job worker returned HTTP %d", response.StatusCode)
	}

	if err = json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		return Snapshot{}, err
	}

	return snapshot, nil
}
