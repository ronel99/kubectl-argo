package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	// Version is the semantic version injected at build time via -ldflags.
	Version = "dev"
	// Commit is the Git SHA injected at build time via -ldflags.
	Commit = "none"
	// BuildDate is the UTC RFC3339 build timestamp injected at build time via -ldflags.
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the kubectl-argo version and build metadata",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("kubectl-argo version %s (commit: %s, built: %s, %s/%s)\n",
			Version, Commit, BuildDate, runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
