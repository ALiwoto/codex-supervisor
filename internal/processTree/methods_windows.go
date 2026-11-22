package processTree

import (
	"fmt"
	"syscall"
	"unsafe"
)

func (process *Process) Wait() (int, error) {
	_, err := syscall.WaitForSingleObject(process.process, syscall.INFINITE)
	if err != nil {
		return -1, err
	}

	var code uint32
	if err = syscall.GetExitCodeProcess(process.process, &code); err != nil {
		return -1, err
	}

	return int(code), nil
}

func (process *Process) Usage() (Usage, error) {
	var counters accounting
	ok, _, err := queryJob.Call(
		uintptr(process.job),
		8,
		uintptr(unsafe.Pointer(&counters)),
		unsafe.Sizeof(counters),
		0,
	)
	if ok == 0 {
		return Usage{}, fmt.Errorf("read process tree counters: %w", err)
	}

	return Usage{
		CPUSeconds: float64(counters.UserTime+counters.KernelTime) / 1e7,
		IOBytes:    counters.IO.ReadBytes + counters.IO.WriteBytes + counters.IO.OtherBytes,
		Processes:  counters.ActiveProcesses,
	}, nil
}

func (process *Process) Kill() error {
	ok, _, err := terminateJob.Call(uintptr(process.job), 1)
	if ok == 0 {
		return fmt.Errorf("terminate process tree: %w", err)
	}

	return nil
}

func (process *Process) Close() {
	syscall.CloseHandle(process.process)
	syscall.CloseHandle(process.job)
}
