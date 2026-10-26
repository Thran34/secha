package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type buildStamp struct {
	version string
	commit  string
	date    string
}

func resolveStamp(ldflags buildStamp, info *debug.BuildInfo) buildStamp {
	out := ldflags
	if info == nil {
		return out
	}
	if out.version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		out.version = strings.TrimPrefix(info.Main.Version, "v")
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if out.commit == "none" && len(s.Value) >= 7 {
				out.commit = s.Value[:7]
			}
		case "vcs.time":
			if out.date == "unknown" && s.Value != "" {
				out.date = s.Value
			}
		}
	}
	return out
}

func versionString() string {
	info, _ := debug.ReadBuildInfo()
	s := resolveStamp(buildStamp{version, commit, date}, info)
	return fmt.Sprintf("secha %s (commit %s, built %s, %s, %s/%s)",
		s.version, s.commit, s.date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the secha version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(versionString())
		},
	}
}
