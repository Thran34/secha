package results

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Thran34/secha/internal/indicator"
)

type Stats struct {
	N      int     `json:"n"`
	Mean   float64 `json:"mean"`
	Stddev float64 `json:"stddev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

type Run struct {
	FaultApplied      bool     `json:"faultApplied"`
	RejectionDetail   string   `json:"rejectionDetail,omitempty"`
	Reachable         bool     `json:"reachable"`
	DataExposed       bool     `json:"dataExposed"`
	Detected          bool     `json:"detected"`
	MTTDMs            *float64 `json:"mttdMs"`
	RecoveryMs        float64  `json:"recoveryMs"`
	RecoveryIntegrity *bool    `json:"recoveryIntegrity"`
}

type Summary struct {
	Runs                  int      `json:"runs"`
	FaultAppliedRate      float64  `json:"faultAppliedRate"`
	ReachableRate         float64  `json:"reachableRate"`
	DataExposedRate       float64  `json:"dataExposedRate"`
	DetectedRate          float64  `json:"detectedRate"`
	MTTDMs                *Stats   `json:"mttdMs"`
	RecoveryMs            Stats    `json:"recoveryMs"`
	RecoveryIntegrityRate *float64 `json:"recoveryIntegrityRate"`
}

type File struct {
	Experiment string  `json:"experiment"`
	Runs       []Run   `json:"runs"`
	Summary    Summary `json:"summary"`
}

func Build(experiment string, reports []*indicator.Report) File {
	f := File{Experiment: experiment}
	for _, r := range reports {
		run := Run{
			FaultApplied:      r.FaultApplied,
			RejectionDetail:   r.RejectionDetail,
			Reachable:         r.Reachable,
			DataExposed:       r.DataExposed,
			Detected:          r.Detected,
			RecoveryMs:        Milliseconds(r.RecoveryLatency),
			RecoveryIntegrity: r.RecoveryIntegrity,
		}
		if r.MTTD != nil {
			ms := Milliseconds(*r.MTTD)
			run.MTTDMs = &ms
		}
		f.Runs = append(f.Runs, run)
	}

	s := indicator.Aggregate(reports)
	f.Summary = Summary{
		Runs:                  s.Runs,
		FaultAppliedRate:      s.FaultAppliedRate,
		ReachableRate:         s.ReachableRate,
		DataExposedRate:       s.DataExposedRate,
		DetectedRate:          s.DetectedRate,
		RecoveryMs:            statsOf(s.Recovery),
		RecoveryIntegrityRate: s.RecoveryIntegrityRate,
	}
	if s.MTTD != nil {
		st := statsOf(*s.MTTD)
		f.Summary.MTTDMs = &st
	}
	return f
}

func Write(w io.Writer, experiment string, reports []*indicator.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(Build(experiment, reports))
}

func Read(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("read results %s: %w", path, err)
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return File{}, fmt.Errorf("parse results %s: %w", path, err)
	}
	if f.Experiment == "" {
		return File{}, fmt.Errorf("parse results %s: missing experiment name, is this a secha run --output json file?", path)
	}
	return f, nil
}

func Milliseconds(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / 1e6
}

func statsOf(s indicator.Stats) Stats {
	return Stats{N: s.N, Mean: s.Mean, Stddev: s.Stddev, Min: s.Min, Max: s.Max}
}
