package processTree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func (process *Process) Wait() (int, error) {
	err := process.command.Wait()
	var exitError *exec.ExitError
	if err != nil && !errors.As(err, &exitError) {
		return -1, err
	}

	return process.command.ProcessState.ExitCode(), nil
}

func (process *Process) Usage() (Usage, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return Usage{}, err
	}

	var result Usage
	group := strconv.Itoa(process.PID)
	for _, entry := range entries {
		if _, err = strconv.Atoi(entry.Name()); err != nil {
			continue
		}

		prefix := "/proc/" + entry.Name()
		data, readError := os.ReadFile(prefix + "/stat")
		if readError != nil {
			continue // A child can exit between enumeration and inspection.
		}

		fields := strings.Fields(string(data)[strings.LastIndexByte(string(data), ')')+1:])
		if len(fields) < 15 || fields[2] != group || fields[0] == "Z" {
			continue
		}
		result.Processes++
		// Linux procfs exposes USER_HZ=100, independently of the kernel timer rate.
		for _, index := range []int{11, 12, 13, 14} {
			ticks, parseError := strconv.ParseUint(fields[index], 10, 64)
			if parseError != nil {
				return Usage{}, fmt.Errorf("parse process CPU: %w", parseError)
			}
			result.CPUSeconds += float64(ticks) / 100
		}
		ioData, readError := os.ReadFile(prefix + "/io")
		if readError != nil {
			continue
		}
		for _, line := range strings.Split(string(ioData), "\n") {
			name, value, found := strings.Cut(line, ":")
			if found && (name == "rchar" || name == "wchar") {
				count, parseError := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
				if parseError == nil {
					result.IOBytes += count
				}
			}
		}
	}
	return result, nil
}

func (process *Process) Kill() error {
	err := syscall.Kill(-process.PID, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}

	return err
}

func (process *Process) Close() {
	_ = process.Kill()
}
