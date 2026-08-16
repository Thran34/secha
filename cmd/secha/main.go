package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/Thran34/secha/internal/coverage"
	"github.com/Thran34/secha/internal/demo"
	"github.com/Thran34/secha/internal/engine"
	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/fault"
	"github.com/Thran34/secha/internal/indicator"
	"github.com/Thran34/secha/internal/k8s"
	"github.com/Thran34/secha/internal/report"
	"github.com/Thran34/secha/internal/results"
	"github.com/Thran34/secha/internal/scaffold"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		fmt.Fprintln(os.Stderr,
			"\ninterrupted: restoring the security control, this may take a moment")
		fmt.Fprintln(os.Stderr,
			"press Ctrl-C again to abort, which WILL LEAVE THE CONTROL DISABLED")
		signal.Stop(signals)
		cancel()
	}()

	root := &cobra.Command{
		Use:   "secha",
		Short: "Security Chaos Engineering experiment runner",
	}
	root.AddCommand(runCmd(), compareCmd(), coverageCmd(), reportCmd(), demoCmd(), initCmd())

	if err := root.ExecuteContext(ctx); err != nil {
		if errors.Is(err, engine.ErrControlLeftDisabled) {
			fmt.Fprintf(os.Stderr, "\n!!! %v\n", err)
		}
		os.Exit(1)
	}
}

func runCmd() *cobra.Command {
	var repeat int
	var output string
	var dryRun bool
	cmd := &cobra.Command{
		Use:          "run <experiment.yaml>",
		Short:        "Run a fault + probe + observation experiment",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
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

			target, err := checkTarget(progress, exp)
			if err != nil {
				return err
			}

			printPlan(progress, exp)
			if repeat > 1 {
				fmt.Fprintf(progress, "  repeat:  %d runs\n", repeat)
			}

			if dryRun {
				printDryRun(progress, exp, target)
				return nil
			}

			var reports []*indicator.Report
			for i := 0; i < repeat; i++ {
				rep, err := engine.Run(cmd.Context(), exp)
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
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be broken, without touching the cluster")
	return cmd
}

func checkTarget(w io.Writer, exp *experiment.Experiment) (k8s.Target, error) {
	target, err := k8s.CurrentTarget()
	if err != nil {
		return target, err
	}
	fmt.Fprintf(w, "cluster: %s\n", target)
	if err := exp.Safety.Check(target); err != nil {
		return target, err
	}
	if !exp.Safety.Pinned() {
		fmt.Fprintf(w, "  note:  no safety.contexts in this experiment, any cluster is accepted\n")
	}
	return target, nil
}

func printDryRun(w io.Writer, exp *experiment.Experiment, target k8s.Target) {
	f, err := fault.New(exp.Fault)
	if err != nil {
		fmt.Fprintf(w, "\ndry run: fault could not be prepared: %v\n", err)
		return
	}
	fmt.Fprintf(w, "\ndry run, nothing was changed on %s\n\n", target)
	fmt.Fprintf(w, "  would break:  %s\n", f.Describe())
	fmt.Fprintf(w, "  would probe:  %s\n", exp.Probe.Type)
	for _, d := range exp.Observe.Detectors {
		fmt.Fprintf(w, "  would watch:  %s\n", d.Type)
	}
	fmt.Fprintf(w, "\nrun again without --dry-run to measure it\n")
}

func compareCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "compare <baseline.yaml> <treatment.yaml>",
		Short:        "Run two experiments and report the resilience delta between them",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseExp, err := experiment.Load(args[0])
			if err != nil {
				return err
			}
			treatExp, err := experiment.Load(args[1])
			if err != nil {
				return err
			}

			for _, exp := range []*experiment.Experiment{baseExp, treatExp} {
				if _, err := checkTarget(os.Stdout, exp); err != nil {
					return err
				}
			}

			fmt.Printf("baseline:  %s\n", baseExp.Metadata.Name)
			baseRep, err := engine.Run(cmd.Context(), baseExp)
			if err != nil {
				return fmt.Errorf("baseline %q: %w", baseExp.Metadata.Name, err)
			}

			fmt.Printf("treatment: %s\n", treatExp.Metadata.Name)
			treatRep, err := engine.Run(cmd.Context(), treatExp)
			if err != nil {
				return fmt.Errorf("treatment %q: %w", treatExp.Metadata.Name, err)
			}

			printComparison(indicator.Compare(baseRep, treatRep))
			return nil
		},
	}
}

func coverageCmd() *cobra.Command {
	var repeat int
	var navigator string
	var platforms []string
	cmd := &cobra.Command{
		Use:          "coverage <experiment.yaml>...",
		Short:        "Run experiments and report measured ATT&CK and D3FEND coverage",
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if repeat < 1 {
				return fmt.Errorf("--repeat must be >= 1, got %d", repeat)
			}

			var observations []coverage.Observation
			for _, path := range args {
				exp, err := experiment.Load(path)
				if err != nil {
					return err
				}
				if _, err := checkTarget(os.Stderr, exp); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "running %s (%d run(s))\n", exp.Metadata.Name, repeat)

				var reports []*indicator.Report
				for i := 0; i < repeat; i++ {
					rep, err := engine.Run(cmd.Context(), exp)
					if err != nil {
						return fmt.Errorf("%s run %d/%d: %w", exp.Metadata.Name, i+1, repeat, err)
					}
					reports = append(reports, rep)
				}
				observations = append(observations, coverage.Observation{Experiment: exp, Reports: reports})
			}

			result := coverage.Build(observations)
			printCoverage(os.Stdout, result)

			if navigator != "" {
				f, err := os.Create(navigator)
				if err != nil {
					return fmt.Errorf("create navigator layer: %w", err)
				}
				defer f.Close()
				if err := coverage.WriteNavigatorLayer(f, "secha measured resilience", platforms, result); err != nil {
					return fmt.Errorf("write navigator layer: %w", err)
				}
				fmt.Fprintf(os.Stderr, "\nnavigator layer written to %s\n", navigator)
				if len(platforms) == 0 {
					fmt.Fprintf(os.Stderr, "matrix not filtered by platform\n")
				} else {
					fmt.Fprintf(os.Stderr, "matrix filtered to platform(s): %s\n", strings.Join(platforms, ", "))
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&repeat, "repeat", 1, "number of times to run each experiment")
	cmd.Flags().StringVar(&navigator, "navigator", "", "write an ATT&CK Navigator layer to this path")
	cmd.Flags().StringSliceVar(&platforms, "platform", []string{coverage.DefaultPlatform},
		"restrict the Navigator matrix to these ATT&CK platforms; pass an empty value to disable the filter")
	return cmd
}

func reportCmd() *cobra.Command {
	var out string
	var svgDir string
	cmd := &cobra.Command{
		Use:          "report <results.json>...",
		Short:        "Render measured results as charts and a summary table",
		Long:         "Reads one or more files written by secha run --output json and renders them as a self-contained HTML report, standalone SVG figures, or both.",
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if out == "" && svgDir == "" {
				return fmt.Errorf("nothing to write: pass --output, --svg-dir, or both")
			}

			var files []results.File
			for _, path := range args {
				f, err := results.Read(path)
				if err != nil {
					return err
				}
				files = append(files, f)
			}

			if out != "" {
				f, err := os.Create(out)
				if err != nil {
					return fmt.Errorf("create report: %w", err)
				}
				defer f.Close()
				if err := report.WriteHTML(f, files); err != nil {
					return fmt.Errorf("write report: %w", err)
				}
				fmt.Fprintf(os.Stderr, "report written to %s\n", out)
			}

			if svgDir != "" {
				written, err := report.WriteSVGs(svgDir, files, report.Light)
				if err != nil {
					return err
				}
				for _, path := range written {
					fmt.Fprintf(os.Stderr, "figure written to %s\n", path)
				}
				fmt.Fprintf(os.Stderr, "figures use the light palette, for embedding in a document\n")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "write a self-contained HTML report to this path")
	cmd.Flags().StringVar(&svgDir, "svg-dir", "", "write each figure as a standalone SVG into this directory")
	return cmd
}

func initCmd() *cobra.Command {
	var namespace, policy, service, out string
	var port int

	cmd := &cobra.Command{
		Use:   "init networkpolicy",
		Short: "Write an experiment for a NetworkPolicy you already run",
		Long: "Reads a NetworkPolicy from the cluster and writes an experiment that deletes it, tries to " +
			"reach what it protects, and puts it back. The probe builds its own pod, so nothing has to be " +
			"deployed first.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "networkpolicy" {
				return fmt.Errorf("unknown kind %q, the only kind so far is networkpolicy", args[0])
			}
			if namespace == "" || policy == "" {
				return fmt.Errorf("--namespace and --policy are required")
			}

			target, err := k8s.CurrentTarget()
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "reading %s/%s from %s\n", namespace, policy, target)

			yaml, err := scaffold.NetworkPolicy(cmd.Context(), scaffold.NetworkPolicyRequest{
				Namespace: namespace,
				Policy:    policy,
				Port:      port,
				Service:   service,
			})
			if err != nil {
				return err
			}

			if out == "" {
				fmt.Print(yaml)
				return nil
			}
			if err := os.WriteFile(out, []byte(yaml), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", out, err)
			}
			fmt.Fprintf(os.Stderr, "written to %s\n", out)
			fmt.Fprintf(os.Stderr, "check it, then: secha run %s --dry-run\n", out)
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace holding the NetworkPolicy")
	cmd.Flags().StringVar(&policy, "policy", "", "name of the NetworkPolicy to test")
	cmd.Flags().StringVar(&service, "service", "", "service to reach; guessed from the policy when omitted")
	cmd.Flags().IntVar(&port, "port", 0, "port to reach; taken from the policy when omitted")
	cmd.Flags().StringVarP(&out, "output", "o", "", "write to this file instead of stdout")
	return cmd
}

func demoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "demo",
		Short: "Stand up a throwaway target, measure one resilience delta, then remove it",
		Long: "Creates a self-contained " + demo.Namespace + " namespace with two redis instances, one " +
			"protected by AUTH and one not, runs the same fault against both, and reports what the AUTH " +
			"layer is worth. Everything it creates is deleted before the command returns.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := k8s.CurrentTarget()
			if err != nil {
				return err
			}
			fmt.Printf("cluster: %s\n\n", target)
			fmt.Printf("secha demo creates the %s namespace, measures, and deletes it again.\n", demo.Namespace)

			runner, err := demo.New(os.Stdout)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			fmt.Printf("setting up the demo target\n")
			if err := runner.Setup(ctx); err != nil {
				teardown(runner, ctx)
				return fmt.Errorf("set up demo: %w", err)
			}
			defer teardown(runner, ctx)

			baseline, treatment, err := demo.Experiments()
			if err != nil {
				return err
			}

			fmt.Printf("\nbaseline:  %s (redis without AUTH)\n", baseline.Metadata.Name)
			baseRep, err := engine.Run(ctx, baseline)
			if err != nil {
				if errors.Is(err, engine.ErrSteadyStateViolated) {
					return errors.Join(err, errNetworkPolicyNotEnforced)
				}
				return fmt.Errorf("baseline %q: %w", baseline.Metadata.Name, err)
			}

			fmt.Printf("treatment: %s (redis with AUTH)\n", treatment.Metadata.Name)
			treatRep, err := engine.Run(ctx, treatment)
			if err != nil {
				return fmt.Errorf("treatment %q: %w", treatment.Metadata.Name, err)
			}

			printComparison(indicator.Compare(baseRep, treatRep))
			printDemoConclusion(os.Stdout, baseRep, treatRep)
			return nil
		},
	}
}

var errNetworkPolicyNotEnforced = errors.New(
	"the attacker reached the target before any fault was injected, which means this cluster " +
		"accepts NetworkPolicy objects but does not enforce them.\n" +
		"The demo needs a CNI that enforces NetworkPolicy — Calico, Cilium, kube-router or Weave.\n" +
		"  minikube:  minikube start --cni=calico\n" +
		"  kind:      install Calico, or use a cluster that ships policy enforcement\n" +
		"  managed:   most cloud providers enforce it by default")

func teardown(runner *demo.Runner, ctx context.Context) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
	defer cancel()
	fmt.Printf("\nremoving the %s namespace\n", demo.Namespace)
	if err := runner.Teardown(cleanupCtx); err != nil {
		fmt.Fprintf(os.Stderr, "\n!!! could not remove the demo namespace: %v\n", err)
		fmt.Fprintf(os.Stderr, "!!! remove it by hand: kubectl delete namespace %s\n", demo.Namespace)
	}
}

func printDemoConclusion(w io.Writer, baseline, treatment *indicator.Report) {
	fmt.Fprintf(w, "\nwhat the AUTH layer was worth:\n\n")
	fmt.Fprintf(w, "  without AUTH  the secret was read: %t, anything detected it: %t\n",
		baseline.DataExposed, baseline.Detected)
	fmt.Fprintf(w, "  with AUTH     the secret was read: %t, anything detected it: %t\n",
		treatment.DataExposed, treatment.Detected)

	if baseline.DataExposed && !baseline.Detected && !treatment.DataExposed && treatment.Detected {
		fmt.Fprintf(w, "\n  Removing AUTH did not only cost the protection. It cost the detection too:\n")
		fmt.Fprintf(w, "  the signal was the failed authentication, and with no AUTH there is none to log.\n")
		fmt.Fprintf(w, "  A dashboard with no alerts would have looked exactly the same.\n")
	}
}

func printCoverage(w io.Writer, r coverage.Result) {
	printTechniques(w, "ATT&CK techniques exercised", r.Attack)
	printTechniques(w, "D3FEND countermeasures involved", r.D3fend)
	if len(r.Unmapped) > 0 {
		fmt.Fprintf(w, "\nunmapped experiments (no attack/d3fend annotation):\n")
		for _, name := range r.Unmapped {
			fmt.Fprintf(w, "  %s\n", name)
		}
	}
}

func printTechniques(w io.Writer, title string, techniques []coverage.Technique) {
	fmt.Fprintf(w, "\n%s:\n\n", title)
	if len(techniques) == 0 {
		fmt.Fprintf(w, "  none\n")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  id\truns\texposed\tdetected\tprevented\tmttd (ms)\tstatus\texperiments")
	for _, t := range techniques {
		mttd := "—"
		if t.MTTD != nil {
			mttd = fmt.Sprintf("%.1f", t.MTTD.Mean)
		}
		fmt.Fprintf(tw, "  %s\t%d\t%.0f%%\t%.0f%%\t%.0f%%\t%s\t%s\t%s\n",
			t.ID, t.Runs, t.ExposedRate*100, t.DetectedRate*100, t.PreventedRate*100,
			mttd, t.Status(), strings.Join(t.Experiments, ", "))
	}
	tw.Flush()
}

func printPlan(w io.Writer, exp *experiment.Experiment) {
	fmt.Fprintf(w, "experiment: %s\n", exp.Metadata.Name)
	if n := len(exp.SteadyState.Checks); n > 0 {
		fmt.Fprintf(w, "  steady:  %d check(s)\n", n)
	}
	fmt.Fprintf(w, "  fault:   %s\n", exp.Fault.Type)
	fmt.Fprintf(w, "  probe:   %s\n", exp.Probe.Type)
	fmt.Fprintf(w, "  observe: %d detector(s), window %s\n",
		len(exp.Observe.Detectors), exp.Observe.Window)
}

func printReport(w io.Writer, r *indicator.Report) {
	fmt.Fprintf(w, "\nreport:\n")
	fmt.Fprintf(w, "  fault applied:    %t\n", r.FaultApplied)
	if !r.FaultApplied && r.RejectionDetail != "" {
		fmt.Fprintf(w, "    rejected by:    %s\n", r.RejectionDetail)
	}
	fmt.Fprintf(w, "  reachable:        %t\n", r.Reachable)
	fmt.Fprintf(w, "  data exposed:     %t\n", r.DataExposed)
	fmt.Fprintf(w, "  detected:         %t\n", r.Detected)
	fmt.Fprintf(w, "  mttd:             %s\n", indicator.FormatMTTD(r.MTTD))
	fmt.Fprintf(w, "  recovery latency: %s\n", r.RecoveryLatency)
	if r.RecoveryIntegrity != nil {
		fmt.Fprintf(w, "  recovery integrity: %t\n", *r.RecoveryIntegrity)
		if r.RecoveryDetail != "" {
			fmt.Fprintf(w, "    violations: %s\n", r.RecoveryDetail)
		}
	}
}

func printSummary(w io.Writer, s indicator.Summary) {
	fmt.Fprintf(w, "\nsummary over %d runs:\n", s.Runs)
	fmt.Fprintf(w, "  fault applied rate: %.0f%%\n", s.FaultAppliedRate*100)
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
	if s.RecoveryIntegrityRate != nil {
		fmt.Fprintf(w, "  recovery integrity rate: %.0f%%\n", *s.RecoveryIntegrityRate*100)
	}
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

func printJSON(w io.Writer, name string, reports []*indicator.Report) error {
	return results.Write(w, name, reports)
}
