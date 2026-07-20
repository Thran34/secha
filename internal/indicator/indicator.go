package indicator

import "time"

type Report struct {
	Experiment      string
	Reachable       bool
	DataExposed     bool
	Detected        bool
	MTTD            *time.Duration
	RecoveryLatency time.Duration
}
