package servicekit

const (
	// RuntimeModeDistributed marks a service process running as its own deployment unit.
	RuntimeModeDistributed = "distributed"
	// RuntimeModeMonolith marks a service embedded in a single-process runtime.
	RuntimeModeMonolith = "monolith"
)
