package processTree

import "os/exec"

type Process struct {
	command *exec.Cmd
	PID     int
}
