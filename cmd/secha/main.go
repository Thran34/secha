package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

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
	return &cobra.Command{
		Use:   "run <experiment.yaml>",
		Short: "Run a fault + probe + observation experiment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exp, err := experiment.Load(args[0])
			if err != nil {
				return err
			}

			printPlan(exp)

			report, err := engine.Run(context.Background(), exp)
			if err != nil {
				return err
			}

			fmt.Printf("\nreport:\n")
			fmt.Printf("  reachable:        %t\n", report.Reachable)
			fmt.Printf("  data exposed:     %t\n", report.DataExposed)
			fmt.Printf("  detected:         %t\n", report.Detected)
			fmt.Printf("  mttd:             %s\n", indicator.FormatMTTD(report.MTTD))
			fmt.Printf("  recovery latency: %s\n", report.RecoveryLatency)
			return nil
		},
	}
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

func printPlan(exp *experiment.Experiment) {
	fmt.Printf("experiment: %s\n", exp.Metadata.Name)
	fmt.Printf("  fault:   %s\n", exp.Fault.Type)
	fmt.Printf("  probe:   %s\n", exp.Probe.Type)
	fmt.Printf("  observe: %d detector(s), window %s\n",
		len(exp.Observe.Detectors), exp.Observe.Window)
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
