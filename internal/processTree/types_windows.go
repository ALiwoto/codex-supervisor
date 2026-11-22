package processTree

import "syscall"

type Process struct {
	process syscall.Handle
	job     syscall.Handle
	PID     int
}

type basicLimit struct {
	ProcessTime        int64
	JobTime            int64
	Flags              uint32
	MinWorkingSet      uintptr
	MaxWorkingSet      uintptr
	ActiveProcessLimit uint32
	Affinity           uintptr
	PriorityClass      uint32
	SchedulingClass    uint32
}

type ioCounters struct {
	ReadOperations  uint64
	WriteOperations uint64
	OtherOperations uint64
	ReadBytes       uint64
	WriteBytes      uint64
	OtherBytes      uint64
}

type extendedLimit struct {
	Basic             basicLimit
	IO                ioCounters
	ProcessMemory     uintptr
	JobMemory         uintptr
	PeakProcessMemory uintptr
	PeakJobMemory     uintptr
}

type accounting struct {
	UserTime            int64
	KernelTime          int64
	PeriodUserTime      int64
	PeriodKernelTime    int64
	PageFaults          uint32
	TotalProcesses      uint32
	ActiveProcesses     uint32
	TerminatedProcesses uint32
	IO                  ioCounters
}
