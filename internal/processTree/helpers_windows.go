package processTree

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

func Start(
	command []string,
	directory string,
	environment []string,
	output *os.File,
) (*Process, error) {
	executable, err := exec.LookPath(command[0])
	if err != nil {
		return nil, err
	}

	application, err := syscall.UTF16PtrFromString(executable)
	if err != nil {
		return nil, err
	}

	arguments := make([]string, len(command))
	for index, argument := range command {
		arguments[index] = syscall.EscapeArg(argument)
	}
	commandLine, err := syscall.UTF16PtrFromString(strings.Join(arguments, " "))
	if err != nil {
		return nil, err
	}

	cwd, err := syscall.UTF16PtrFromString(directory)
	if err != nil {
		return nil, err
	}

	sort.Slice(environment, func(first, second int) bool {
		return strings.ToUpper(environment[first]) < strings.ToUpper(environment[second])
	})
	envBlock := utf16.Encode([]rune(strings.Join(environment, "\x00") + "\x00\x00"))
	jobHandle, _, callError := createJob.Call(0, 0)
	if jobHandle == 0 {
		return nil, fmt.Errorf("create process job: %w", callError)
	}

	job := syscall.Handle(jobHandle)
	limits := extendedLimit{
		Basic: basicLimit{
			Flags: 0x2000,
		},
	} // KILL_ON_JOB_CLOSE
	ok, _, callError := setJobInfo.Call(
		jobHandle,
		9,
		uintptr(unsafe.Pointer(&limits)),
		unsafe.Sizeof(limits),
	)
	if ok == 0 {
		syscall.CloseHandle(job)
		return nil, fmt.Errorf("set process job limits: %w", callError)
	}

	nullInput, err := os.Open(os.DevNull)
	if err != nil {
		syscall.CloseHandle(job)
		return nil, err
	}

	defer nullInput.Close()
	inputHandle := syscall.Handle(nullInput.Fd())
	outputHandle := syscall.Handle(output.Fd())
	if err = syscall.SetHandleInformation(
		inputHandle,
		syscall.HANDLE_FLAG_INHERIT,
		syscall.HANDLE_FLAG_INHERIT,
	); err != nil {
		syscall.CloseHandle(job)
		return nil, err
	}

	if err = syscall.SetHandleInformation(
		outputHandle,
		syscall.HANDLE_FLAG_INHERIT,
		syscall.HANDLE_FLAG_INHERIT,
	); err != nil {
		syscall.CloseHandle(job)
		return nil, err
	}

	defer syscall.SetHandleInformation(outputHandle, syscall.HANDLE_FLAG_INHERIT, 0)
	startup := syscall.StartupInfo{
		Cb:        uint32(unsafe.Sizeof(syscall.StartupInfo{})),
		Flags:     syscall.STARTF_USESTDHANDLES,
		StdInput:  inputHandle,
		StdOutput: outputHandle,
		StdErr:    outputHandle,
	}
	var information syscall.ProcessInformation
	// Suspend before any user code: descendants must inherit membership in our job.
	err = syscall.CreateProcess(application, commandLine, nil, nil, true,
		0x4|0x400|0x08000000, &envBlock[0], cwd, &startup, &information)
	if err != nil {
		syscall.CloseHandle(job)
		return nil, err
	}

	defer syscall.CloseHandle(information.Thread)
	ok, _, callError = assignJob.Call(jobHandle, uintptr(information.Process))
	if ok == 0 {
		syscall.TerminateProcess(information.Process, 1)
		syscall.CloseHandle(information.Process)
		syscall.CloseHandle(job)
		return nil, fmt.Errorf("assign process job: %w", callError)
	}

	resumed, _, callError := resumeThread.Call(uintptr(information.Thread))
	if uint32(resumed) == 0xffffffff {
		syscall.CloseHandle(job)
		syscall.CloseHandle(information.Process)
		return nil, fmt.Errorf("resume process: %w", callError)
	}

	return &Process{
		process: information.Process,
		job:     job,
		PID:     int(information.ProcessId),
	}, nil
}

func Detach(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000 | 0x200,
		HideWindow:    true,
	}
}
