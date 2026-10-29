package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"kubectl-argo/internal/argocd"
	"kubectl-argo/internal/config"
)

type contextKey string

const (
	clientContextKey      contextKey = "argocd_client"
	resolvedAppContextKey contextKey = "resolved_app"
)

var (
	flagNamespace    string
	flagArgoConfig   string
	flagRefreshCache bool
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "kubectl-argo",
	Short: "kubectl wrapper powered by the Argo CD API and RBAC",
	Long: `kubectl-argo translates standard kubectl commands (get, logs, exec) into
Argo CD API calls without requiring a kubeconfig or direct Kubernetes cluster access.

It silently intercepts the -n/--namespace flag and resolves it to the underlying
Argo CD Application name via the Resolution Engine.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip authentication and namespace resolution for built-in utility commands.
		if isUtilityCommand(cmd) {
			return nil
		}

		cfg, err := config.Load(flagArgoConfig)
		if err != nil {
			return err
		}

		client, err := argocd.NewClient(cfg)
		if err != nil {
			return err
		}

		resolver := argocd.NewResolver(
			client,
			argocd.WithForceRefresh(flagRefreshCache),
		)

		resolvedApp, err := resolver.ResolveNamespace(cmd.Context(), flagNamespace)
		if err != nil {
			return err
		}

		ctx := context.WithValue(cmd.Context(), clientContextKey, client)
		ctx = context.WithValue(ctx, resolvedAppContextKey, resolvedApp)
		cmd.SetContext(ctx)

		return nil
	},
}

// Execute runs the root Cobra command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	pf := rootCmd.PersistentFlags()

	pf.StringVarP(&flagNamespace, "namespace", "n", "default", "Kubernetes namespace (silently resolved to the target Argo CD Application)")
	pf.StringVar(&flagArgoConfig, "argocd-config", "", "Path to Argo CD config file (default \"~/.argocd/config\")")
	pf.BoolVar(&flagRefreshCache, "refresh-cache", false, "Bypass local namespace cache and refresh from Argo CD ApplicationService.List")

	pf.String("server", "", "Argo CD server address (overrides ARGOCD_SERVER and ~/.argocd/config)")
	pf.String("auth-token", "", "Argo CD authentication token (overrides ARGOCD_AUTH_TOKEN and ~/.argocd/config)")
	pf.Bool("insecure", false, "Skip server certificate and domain verification")
	pf.Bool("grpc-web", true, "Enable gRPC-Web protocol for Argo CD API calls")
	pf.Bool("plaintext", false, "Disable TLS when connecting to Argo CD server")

	_ = viper.BindPFlag("server", pf.Lookup("server"))
	_ = viper.BindPFlag("auth-token", pf.Lookup("auth-token"))
	_ = viper.BindPFlag("insecure", pf.Lookup("insecure"))
	_ = viper.BindPFlag("grpc-web", pf.Lookup("grpc-web"))
	_ = viper.BindPFlag("plaintext", pf.Lookup("plaintext"))
}

// ClientFromContext retrieves the initialized Argo CD Client from the command context.
func ClientFromContext(ctx context.Context) (*argocd.Client, error) {
	client, ok := ctx.Value(clientContextKey).(*argocd.Client)
	if !ok || client == nil {
		return nil, fmt.Errorf("Argo CD client not initialized in command context")
	}
	return client, nil
}

// ResolvedAppFromContext retrieves the ResolvedApp mapped from the -n/--namespace flag.
func ResolvedAppFromContext(ctx context.Context) (*argocd.ResolvedApp, error) {
	app, ok := ctx.Value(resolvedAppContextKey).(*argocd.ResolvedApp)
	if !ok || app == nil {
		return nil, fmt.Errorf("resolved Argo CD application not found in command context")
	}
	return app, nil
}

func isUtilityCommand(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "help", "version", "completion", "__complete", "__completeNoDesc":
		return true
	default:
		return false
	}
}
