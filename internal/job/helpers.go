package job

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func DefaultRoot() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "codex-supervisor", "jobs"), nil
}

func randomID() string {
	return rand.Text()
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 80 {
		return false
	}
	for _, character := range id {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

func writeJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), ".state-*")
	if err != nil {
		return err
	}

	name := temporary.Name()
	defer os.Remove(name)
	if _, err = temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}

	if err = temporary.Close(); err != nil {
		return err
	}

	return os.Rename(name, path)
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, value)
}

func terminal(state string) bool {
	return state != "starting" && state != "running"
}

func diagnostics(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}

	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil
	}

	if info.Size() > 64*1024 {
		_, _ = file.Seek(-64*1024, io.SeekEnd)
	}

	data, err := io.ReadAll(io.LimitReader(file, 64*1024))
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.ToValidUTF8(string(data), "�"), "\n")
	result := make([]string, 0, 12)
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") ||
			strings.Contains(lower, "failed") {
			result = append(result, boundedLine(line))
		}
	}
	if len(result) == 0 {
		for _, line := range lines[max(0, len(lines)-8):] {
			if line != "" {
				result = append(result, boundedLine(line))
			}
		}
	}

	return result[max(0, len(result)-12):]
}

func boundedLine(line string) string {
	runes := []rune(strings.TrimSuffix(line, "\r"))
	if len(runes) > 500 {
		return string(runes[:500]) + "…"
	}

	return string(runes)
}
