package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func versionString() string {
	return fmt.Sprintf("secha %s (commit %s, built %s, %s, %s/%s)",
		version, commit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
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
