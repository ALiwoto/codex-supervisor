package job

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"
)

func (current *worker) HandleRequest(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.Header.Get("Origin") != "" ||
		subtle.ConstantTimeCompare(
			[]byte(request.Header.Get("Authorization")),
			[]byte("Bearer "+current.connection.Token),
		) != 1 {
		http.Error(response, "unauthorized", http.StatusUnauthorized)
		return
	}

	action := request.URL.Path
	if action != "/wait" && action != "/status" && action != "/cancel" {
		http.NotFound(response, request)
		return
	}

	if action == "/cancel" {
		current.cancelOnce.Do(func() {
			close(current.cancel)
		})
	}

	ignoreWarning := request.URL.Query().Get("after_warning") == "true"
	for {
		current.mu.Lock()
		snapshot := current.snapshot
		changed := current.changed
		current.mu.Unlock()
		if !terminal(snapshot.State) {
			snapshot.ElapsedSeconds = time.Since(snapshot.Started).Seconds()
		}

		if terminal(snapshot.State) || action == "/status" ||
			(action == "/wait" && snapshot.Warning != "" && !ignoreWarning) {
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(snapshot)
			return
		}

		if snapshot.Warning == "" {
			ignoreWarning = false
		}
		select {
		case <-request.Context().Done():
			return // Cancelling a wait never cancels the durable job.
		case <-changed:
		}
	}
}
