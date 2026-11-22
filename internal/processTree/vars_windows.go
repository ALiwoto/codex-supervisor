package processTree

import "syscall"

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	createJob    = kernel32.NewProc("CreateJobObjectW")
	setJobInfo   = kernel32.NewProc("SetInformationJobObject")
	assignJob    = kernel32.NewProc("AssignProcessToJobObject")
	queryJob     = kernel32.NewProc("QueryInformationJobObject")
	terminateJob = kernel32.NewProc("TerminateJobObject")
	resumeThread = kernel32.NewProc("ResumeThread")
)
