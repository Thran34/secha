package indicator

import "time"

type Report struct {
	Experiment        string
	FaultApplied      bool
	RejectionDetail   string
	Reachable         bool
	DataExposed       bool
	Detected          bool
	MTTD              *time.Duration
	RecoveryLatency   time.Duration
	RecoveryIntegrity *bool
	RecoveryDetail    string
}
