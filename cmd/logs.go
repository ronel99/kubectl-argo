package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"kubectl-argo/internal/argocd"
)

var (
	flagLogsFollow     bool
	flagLogsTail       int64
	flagLogsContainer  string
	flagLogsPrevious   bool
	flagLogsTimestamps bool
)

var logsCmd = &cobra.Command{
	Use:   "logs [-f] [-c CONTAINER] [--tail LINES] (POD | pod/POD)",
	Short: "Print the logs for a container in a pod via the Argo CD PodLogs API",
	Long: `Streams or fetches container logs for a specific pod within the resolved
Argo CD Application without requiring direct Kubernetes API access.`,
	Example: `  # Return snapshot logs from pod web-6d4f8b in namespace payments-prod
  kubectl-argo logs web-6d4f8b -n payments-prod

  # Stream live logs (-f) showing only the most recent 100 lines (--tail=100)
  kubectl-argo logs -f --tail=100 web-6d4f8b -n payments-prod

  # Stream logs from a specific container in a multi-container pod
  kubectl-argo logs -f -c istio-proxy web-6d4f8b -n payments-prod`,
	Args: cobra.ExactArgs(1),
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&flagLogsFollow, "follow", "f", false, "Specify if the logs should be streamed")
	logsCmd.Flags().Int64Var(&flagLogsTail, "tail", -1, "Lines of recent log file to display. Defaults to -1, showing all log lines")
	logsCmd.Flags().StringVarP(&flagLogsContainer, "container", "c", "", "Print the logs of this container")
	logsCmd.Flags().BoolVarP(&flagLogsPrevious, "previous", "p", false, "Print the logs for the previous instance of the container in a pod if it exists")
	logsCmd.Flags().BoolVar(&flagLogsTimestamps, "timestamps", false, "Include timestamps on each line in the log output")

	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	podName, err := parsePodTarget(args[0])
	if err != nil {
		return err
	}

	client, err := ClientFromContext(cmd.Context())
	if err != nil {
		return err
	}

	resolvedApp, err := ResolvedAppFromContext(cmd.Context())
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := argocd.PodLogsOptions{
		PodName:    podName,
		Container:  strings.TrimSpace(flagLogsContainer),
		Follow:     flagLogsFollow,
		TailLines:  flagLogsTail,
		Previous:   flagLogsPrevious,
		Timestamps: flagLogsTimestamps,
	}

	return client.StreamPodLogs(ctx, resolvedApp, opts, os.Stdout)
}

// parsePodTarget accepts either "<pod-name>" or "pod/<pod-name>" / "pods/<pod-name>".
func parsePodTarget(raw string) (string, error) {
	target := strings.TrimSpace(raw)
	if strings.Contains(target, "/") {
		parts := strings.SplitN(target, "/", 2)
		prefix := strings.ToLower(strings.TrimSpace(parts[0]))
		if prefix != "po" && prefix != "pod" && prefix != "pods" {
			return "", fmt.Errorf("unsupported resource prefix %q in %q: logs only supports pods", parts[0], raw)
		}
		target = strings.TrimSpace(parts[1])
	}
	if target == "" {
		return "", fmt.Errorf("pod name cannot be empty")
	}
	return target, nil
}
