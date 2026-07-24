package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"secha/internal/engine"
	"secha/internal/experiment"
	"secha/internal/indicator"
)

func main() {
	root := &cobra.Command{
		Use:   "secha",
		Short: "Security Chaos Engineering experiment runner",
	}
	root.AddCommand(runCmd(), compareCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runCmd() *cobra.Command {
	var repeat int
	var output string
	cmd := &cobra.Command{
		Use:   "run <experiment.yaml>",
		Short: "Run a fault + probe + observation experiment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if repeat < 1 {
				return fmt.Errorf("--repeat must be >= 1, got %d", repeat)
			}
			if output != "text" && output != "json" {
				return fmt.Errorf("--output must be text or json, got %q", output)
			}

			exp, err := experiment.Load(args[0])
			if err != nil {
				return err
			}

			jsonOut := output == "json"
			progress := io.Writer(os.Stdout)
			if jsonOut {
				progress = os.Stderr
			}

			printPlan(progress, exp)
			if repeat > 1 {
				fmt.Fprintf(progress, "  repeat:  %d runs\n", repeat)
			}

			var reports []*indicator.Report
			for i := 0; i < repeat; i++ {
				rep, err := engine.Run(context.Background(), exp)
				if err != nil {
					return fmt.Errorf("run %d/%d: %w", i+1, repeat, err)
				}
				reports = append(reports, rep)
				if repeat > 1 {
					fmt.Fprintf(progress, "  run %2d/%d: detected=%t mttd=%s\n",
						i+1, repeat, rep.Detected, indicator.FormatMTTD(rep.MTTD))
				}
			}

			if jsonOut {
				return printJSON(os.Stdout, exp.Metadata.Name, reports)
			}
			if repeat == 1 {
				printReport(os.Stdout, reports[0])
			} else {
				printSummary(os.Stdout, indicator.Aggregate(reports))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&repeat, "repeat", 1, "number of times to run the experiment")
	cmd.Flags().StringVar(&output, "output", "text", "output format: text or json")
	return cmd
}

func compareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compare <baseline.yaml> <treatment.yaml>",
		Short: "Run two experiments and report the resilience delta between them",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			baseExp, err := experiment.Load(args[0])
			if err != nil {
				return err
			}
			treatExp, err := experiment.Load(args[1])
			if err != nil {
				return err
			}

			fmt.Printf("baseline:  %s\n", baseExp.Metadata.Name)
			baseRep, err := engine.Run(context.Background(), baseExp)
			if err != nil {
				return fmt.Errorf("baseline %q: %w", baseExp.Metadata.Name, err)
			}

			fmt.Printf("treatment: %s\n", treatExp.Metadata.Name)
			treatRep, err := engine.Run(context.Background(), treatExp)
			if err != nil {
				return fmt.Errorf("treatment %q: %w", treatExp.Metadata.Name, err)
			}

			printComparison(indicator.Compare(baseRep, treatRep))
			return nil
		},
	}
}

func printPlan(w io.Writer, exp *experiment.Experiment) {
	fmt.Fprintf(w, "experiment: %s\n", exp.Metadata.Name)
	fmt.Fprintf(w, "  fault:   %s\n", exp.Fault.Type)
	fmt.Fprintf(w, "  probe:   %s\n", exp.Probe.Type)
	fmt.Fprintf(w, "  observe: %d detector(s), window %s\n",
		len(exp.Observe.Detectors), exp.Observe.Window)
}

func printReport(w io.Writer, r *indicator.Report) {
	fmt.Fprintf(w, "\nreport:\n")
	fmt.Fprintf(w, "  reachable:        %t\n", r.Reachable)
	fmt.Fprintf(w, "  data exposed:     %t\n", r.DataExposed)
	fmt.Fprintf(w, "  detected:         %t\n", r.Detected)
	fmt.Fprintf(w, "  mttd:             %s\n", indicator.FormatMTTD(r.MTTD))
	fmt.Fprintf(w, "  recovery latency: %s\n", r.RecoveryLatency)
}

func printSummary(w io.Writer, s indicator.Summary) {
	fmt.Fprintf(w, "\nsummary over %d runs:\n", s.Runs)
	fmt.Fprintf(w, "  reachable rate:    %.0f%%\n", s.ReachableRate*100)
	fmt.Fprintf(w, "  data exposed rate: %.0f%%\n", s.DataExposedRate*100)
	fmt.Fprintf(w, "  detected rate:     %.0f%%\n", s.DetectedRate*100)
	if s.MTTD != nil {
		fmt.Fprintf(w, "  mttd (ms):         mean %.1f  sd %.1f  min %.1f  max %.1f  (n=%d)\n",
			s.MTTD.Mean, s.MTTD.Stddev, s.MTTD.Min, s.MTTD.Max, s.MTTD.N)
	} else {
		fmt.Fprintf(w, "  mttd (ms):         never detected\n")
	}
	fmt.Fprintf(w, "  recovery (ms):     mean %.1f  sd %.1f  min %.1f  max %.1f\n",
		s.Recovery.Mean, s.Recovery.Stddev, s.Recovery.Min, s.Recovery.Max)
}

func printComparison(c indicator.Comparison) {
	fmt.Printf("\nresilience delta: %s → %s\n\n", c.Baseline.Experiment, c.Treatment.Experiment)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  indicator\tbaseline\ttreatment\tverdict")
	for _, f := range c.Fields() {
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", f.Name, f.Baseline, f.Treatment, f.Verdict)
	}
	w.Flush()
}

type runJSON struct {
	Reachable   bool     `json:"reachable"`
	DataExposed bool     `json:"dataExposed"`
	Detected    bool     `json:"detected"`
	MTTDMs      *float64 `json:"mttdMs"`
	RecoveryMs  float64  `json:"recoveryMs"`
}

type statsJSON struct {
	N      int     `json:"n"`
	Mean   float64 `json:"mean"`
	Stddev float64 `json:"stddev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

type summaryJSON struct {
	Runs            int        `json:"runs"`
	ReachableRate   float64    `json:"reachableRate"`
	DataExposedRate float64    `json:"dataExposedRate"`
	DetectedRate    float64    `json:"detectedRate"`
	MTTDMs          *statsJSON `json:"mttdMs"`
	RecoveryMs      statsJSON  `json:"recoveryMs"`
}

type reportJSON struct {
	Experiment string      `json:"experiment"`
	Runs       []runJSON   `json:"runs"`
	Summary    summaryJSON `json:"summary"`
}

func printJSON(w io.Writer, name string, reports []*indicator.Report) error {
	out := reportJSON{Experiment: name}
	for _, r := range reports {
		rj := runJSON{
			Reachable:   r.Reachable,
			DataExposed: r.DataExposed,
			Detected:    r.Detected,
			RecoveryMs:  durMs(r.RecoveryLatency),
		}
		if r.MTTD != nil {
			ms := durMs(*r.MTTD)
			rj.MTTDMs = &ms
		}
		out.Runs = append(out.Runs, rj)
	}

	s := indicator.Aggregate(reports)
	out.Summary = summaryJSON{
		Runs:            s.Runs,
		ReachableRate:   s.ReachableRate,
		DataExposedRate: s.DataExposedRate,
		DetectedRate:    s.DetectedRate,
		RecoveryMs:      toStatsJSON(s.Recovery),
	}
	if s.MTTD != nil {
		sj := toStatsJSON(*s.MTTD)
		out.Summary.MTTDMs = &sj
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func toStatsJSON(s indicator.Stats) statsJSON {
	return statsJSON{N: s.N, Mean: s.Mean, Stddev: s.Stddev, Min: s.Min, Max: s.Max}
}

func durMs(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / 1e6
}
