package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"secha/internal/engine"
	"secha/internal/experiment"
)

func main() {
	root := &cobra.Command{
		Use:   "secha",
		Short: "Security Chaos Engineering experiment runner",
	}
	root.AddCommand(runCmd())

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

			fmt.Printf("experiment: %s\n", exp.Metadata.Name)
			fmt.Printf("  fault:   %s\n", exp.Fault.Type)
			fmt.Printf("  probe:   %s\n", exp.Probe.Type)
			fmt.Printf("  observe: %d detector(s), window %s\n",
				len(exp.Observe.Detectors), exp.Observe.Window)

			report, err := engine.Run(context.Background(), exp)
			if err != nil {
				return err
			}

			fmt.Printf("\nreport:\n")
			fmt.Printf("  reachable:        %t\n", report.Reachable)
			fmt.Printf("  data exposed:     %t\n", report.DataExposed)
			fmt.Printf("  detected:         %t\n", report.Detected)
			if report.MTTD != nil {
				fmt.Printf("  mttd:             %s\n", *report.MTTD)
			}
			fmt.Printf("  recovery latency: %s\n", report.RecoveryLatency)
			return nil
		},
	}
}
