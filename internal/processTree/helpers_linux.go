package processTree

import (
	"os"
	"os/exec"
	"syscall"
)

func Start(
	command []string,
	directory string,
	environment []string,
	output *os.File,
) (*Process, error) {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = directory
	cmd.Env = environment
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &Process{
		command: cmd,
		PID:     cmd.Process.Pid,
	}, nil
}

func Detach(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
