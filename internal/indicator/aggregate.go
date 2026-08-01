package indicator

import (
	"math"
	"time"
)

type Stats struct {
	N      int
	Mean   float64
	Stddev float64
	Min    float64
	Max    float64
}

type Summary struct {
	Runs                  int
	FaultAppliedRate      float64
	ReachableRate         float64
	DataExposedRate       float64
	DetectedRate          float64
	MTTD                  *Stats
	Recovery              Stats
	RecoveryIntegrityRate *float64
}

func Aggregate(reports []*Report) Summary {
	s := Summary{Runs: len(reports)}
	if len(reports) == 0 {
		return s
	}

	var applied, reachable, exposed, detected int
	var integrityChecked, integrityIntact int
	var mttdMs, recoveryMs []float64
	for _, r := range reports {
		if r.FaultApplied {
			applied++
		}
		if r.Reachable {
			reachable++
		}
		if r.DataExposed {
			exposed++
		}
		if r.Detected {
			detected++
		}
		if r.MTTD != nil {
			mttdMs = append(mttdMs, durMs(*r.MTTD))
		}
		if r.RecoveryIntegrity != nil {
			integrityChecked++
			if *r.RecoveryIntegrity {
				integrityIntact++
			}
		}
		recoveryMs = append(recoveryMs, durMs(r.RecoveryLatency))
	}

	n := float64(len(reports))
	s.FaultAppliedRate = float64(applied) / n
	s.ReachableRate = float64(reachable) / n
	s.DataExposedRate = float64(exposed) / n
	s.DetectedRate = float64(detected) / n
	if len(mttdMs) > 0 {
		st := computeStats(mttdMs)
		s.MTTD = &st
	}
	if integrityChecked > 0 {
		rate := float64(integrityIntact) / float64(integrityChecked)
		s.RecoveryIntegrityRate = &rate
	}
	s.Recovery = computeStats(recoveryMs)
	return s
}

func computeStats(xs []float64) Stats {
	st := Stats{N: len(xs), Min: xs[0], Max: xs[0]}
	var sum float64
	for _, x := range xs {
		sum += x
		if x < st.Min {
			st.Min = x
		}
		if x > st.Max {
			st.Max = x
		}
	}
	st.Mean = sum / float64(len(xs))
	if len(xs) > 1 {
		var ss float64
		for _, x := range xs {
			d := x - st.Mean
			ss += d * d
		}
		st.Stddev = math.Sqrt(ss / float64(len(xs)-1))
	}
	return st
}

func durMs(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / 1e6
}
