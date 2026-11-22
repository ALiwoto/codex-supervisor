package processTree

// Usage is cumulative for a process tree, including children where the OS retains them.
type Usage struct {
	CPUSeconds float64 `json:"cpu_seconds"`
	IOBytes    uint64  `json:"io_bytes"`
	Processes  uint32  `json:"processes"`
}
